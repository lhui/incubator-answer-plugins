/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package tencentyuncos

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/apache/incubator-answer-plugins/util"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/apache/incubator-answer-plugins/storage-tencentyuncos/i18n"
	"github.com/apache/incubator-answer/plugin"
	"github.com/tencentyun/cos-go-sdk-v5"
)

//go:embed  info.yaml
var Info embed.FS

const (
	// 10MB
	defaultMaxFileSize int64 = 10 * 1024 * 1024
)

type Storage struct {
	Config *StorageConfig
}

type StorageConfig struct {
	Region          string `json:"region"`
	BucketName      string `json:"bucket_name"`
	ObjectKeyPrefix string `json:"object_key_prefix"`
	SecretID        string `json:"secret_id"`
	SecretKey       string `json:"secret_key"`
	VisitUrlPrefix  string `json:"visit_url_prefix"`
	MaxFileSize     string `json:"max_file_size"`
}

func init() {
	plugin.Register(&Storage{
		Config: &StorageConfig{},
	})
}

func (s *Storage) Info() plugin.Info {
	info := &util.Info{}
	info.GetInfo(Info)

	return plugin.Info{
		Name:        plugin.MakeTranslator(i18n.InfoName),
		SlugName:    info.SlugName,
		Description: plugin.MakeTranslator(i18n.InfoDescription),
		Author:      info.Author,
		Version:     info.Version,
		Link:        info.Link,
	}
}

func (s *Storage) UploadFile(ctx *plugin.GinContext, source plugin.UploadSource) (resp plugin.UploadFileResponse) {
	resp = plugin.UploadFileResponse{}

	BucketURL, _ := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", s.Config.BucketName, s.Config.Region))
	BaseURL := &cos.BaseURL{BucketURL: BucketURL}
	client := cos.NewClient(BaseURL, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  s.Config.SecretID,
			SecretKey: s.Config.SecretKey,
		},
	})

	_, err := client.Bucket.IsExist(ctx)
	if err != nil {
		resp.OriginalError = fmt.Errorf("head bucket failed: %v", err)
		resp.DisplayErrorMsg = plugin.MakeTranslator(i18n.ErrMisStorageConfig)
		return resp
	}

	file, err := ctx.FormFile("file")
	if err != nil {
		resp.OriginalError = fmt.Errorf("get upload file failed: %v", err)
		resp.DisplayErrorMsg = plugin.MakeTranslator(i18n.ErrFileNotFound)
		return resp
	}

	if !s.CheckFileType(file.Filename, source) {
		resp.OriginalError = fmt.Errorf("file type not allowed")
		resp.DisplayErrorMsg = plugin.MakeTranslator(i18n.ErrUnsupportedFileType)
		return resp
	}

	if file.Size > s.maxFileSizeLimit() {
		resp.OriginalError = fmt.Errorf("file size too large")
		resp.DisplayErrorMsg = plugin.MakeTranslator(i18n.ErrOverFileSizeLimit)
		return resp
	}

	openFile, err := file.Open()
	if err != nil {
		resp.OriginalError = fmt.Errorf("get file failed: %v", err)
		resp.DisplayErrorMsg = plugin.MakeTranslator(i18n.ErrFileNotFound)
		return resp
	}
	defer openFile.Close()

	objectKey := s.createObjectKey(file.Filename, source)
	_, err = client.Object.Put(ctx, objectKey, openFile, nil)
	if err != nil {
		resp.OriginalError = fmt.Errorf("upload file failed: %v", err)
		resp.DisplayErrorMsg = plugin.MakeTranslator(i18n.ErrUploadFileFailed)
		return resp
	}
	resp.FullURL = s.Config.VisitUrlPrefix + objectKey
	return resp
}

func (s *Storage) createObjectKey(originalFilename string, source plugin.UploadSource) string {
	ext := strings.ToLower(filepath.Ext(originalFilename))
	randomString := s.randomObjectKey()
	switch source {
	case plugin.UserAvatar:
		return s.Config.ObjectKeyPrefix + "avatar/" + randomString + ext
	case plugin.UserPost:
		return s.Config.ObjectKeyPrefix + "post/" + randomString + ext
	case plugin.AdminBranding:
		return s.Config.ObjectKeyPrefix + "branding/" + randomString + ext
	default:
		return s.Config.ObjectKeyPrefix + "other/" + randomString + ext
	}
}

func (s *Storage) randomObjectKey() string {
	bytes := make([]byte, 4)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%d", time.Now().UnixNano()) + hex.EncodeToString(bytes)
}

func (s *Storage) CheckFileType(originalFilename string, source plugin.UploadSource) bool {
	ext := strings.ToLower(filepath.Ext(originalFilename))
	if _, ok := plugin.DefaultFileTypeCheckMapping[source][ext]; ok {
		return true
	}
	return false
}

func (s *Storage) maxFileSizeLimit() int64 {
	if len(s.Config.MaxFileSize) == 0 {
		return defaultMaxFileSize
	}
	limit, _ := strconv.Atoi(s.Config.MaxFileSize)
	if limit <= 0 {
		return defaultMaxFileSize
	}
	return int64(limit) * 1024 * 1024
}

func (s *Storage) ConfigFields() []plugin.ConfigField {
	return []plugin.ConfigField{
		{
			Name:        "region",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigRegionTitle),
			Description: plugin.MakeTranslator(i18n.ConfigRegionDescription),
			Required:    true,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeText,
			},
			Value: s.Config.Region,
		},
		{
			Name:        "bucket_name",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigBucketNameTitle),
			Description: plugin.MakeTranslator(i18n.ConfigBucketNameDescription),
			Required:    true,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeText,
			},
			Value: s.Config.BucketName,
		},
		{
			Name:        "object_key_prefix",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigObjectKeyPrefixTitle),
			Description: plugin.MakeTranslator(i18n.ConfigObjectKeyPrefixDescription),
			Required:    false,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeText,
			},
			Value: s.Config.ObjectKeyPrefix,
		},
		{
			Name:        "secret_id",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigSecretIdTitle),
			Description: plugin.MakeTranslator(i18n.ConfigSecretIdDescription),
			Required:    true,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeText,
			},
			Value: s.Config.SecretID,
		},
		{
			Name:        "secret_key",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigSecretKeyTitle),
			Description: plugin.MakeTranslator(i18n.ConfigSecretKeyDescription),
			Required:    true,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeText,
			},
			Value: s.Config.SecretKey,
		},
		{
			Name:        "visit_url_prefix",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigVisitUrlPrefixTitle),
			Description: plugin.MakeTranslator(i18n.ConfigVisitUrlPrefixDescription),
			Required:    true,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeText,
			},
			Value: s.Config.VisitUrlPrefix,
		},
		{
			Name:        "max_file_size",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigMaxFileSizeTitle),
			Description: plugin.MakeTranslator(i18n.ConfigMaxFileSizeDescription),
			Required:    false,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeNumber,
			},
			Value: s.Config.MaxFileSize,
		},
	}
}

func (s *Storage) ConfigReceiver(config []byte) error {
	c := &StorageConfig{}
	_ = json.Unmarshal(config, c)
	s.Config = c
	return nil
}