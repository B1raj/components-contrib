/*
Copyright 2021 The Dapr Authors
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package secretmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go/service/secretsmanager"

	awsAuth "github.com/dapr/components-contrib/common/authentication/aws"
	"github.com/dapr/components-contrib/metadata"
	"github.com/dapr/components-contrib/secretstores"
	"github.com/dapr/kit/logger"
)

const (
	VersionID    = "version_id"
	VersionStage = "version_stage"
)

var _ secretstores.SecretStore = (*smSecretStore)(nil)

// NewSecretManager returns a new secret manager store.
func NewSecretManager(logger logger.Logger) secretstores.SecretStore {
	return &smSecretStore{logger: logger}
}

type SecretManagerMetaData struct {
	Region       string `json:"region" mapstructure:"region" mdignore:"true"`
	AccessKey    string `json:"accessKey" mapstructure:"accessKey" mdignore:"true"`
	SecretKey    string `json:"secretKey" mapstructure:"secretKey" mdignore:"true"`
	SessionToken string `json:"sessionToken" mapstructure:"sessionToken" mdignore:"true"`
	Endpoint     string `json:"endpoint" mapstructure:"endpoint"`
}

type smSecretStore struct {
	authProvider awsAuth.Provider
	logger       logger.Logger
}

// Init creates an AWS secret manager client.
func (s *smSecretStore) Init(ctx context.Context, metadata secretstores.Metadata) error {
	meta, err := s.getSecretManagerMetadata(metadata)
	if err != nil {
		return err
	}

	opts := awsAuth.Options{
		Logger:       s.logger,
		Region:       meta.Region,
		AccessKey:    meta.AccessKey,
		SecretKey:    meta.SecretKey,
		SessionToken: meta.SessionToken,
		Endpoint:     meta.Endpoint,
	}

	provider, err := awsAuth.NewProvider(ctx, opts, awsAuth.GetConfig(opts))
	if err != nil {
		return err
	}
	s.authProvider = provider
	return nil
}

// GetSecret retrieves a secret using a key and returns a map of decrypted string/string values.
func (s *smSecretStore) GetSecret(ctx context.Context, req secretstores.GetSecretRequest) (secretstores.GetSecretResponse, error) {
	var versionID *string
	if value, ok := req.Metadata[VersionID]; ok {
		versionID = &value
	}
	var versionStage *string
	if value, ok := req.Metadata[VersionStage]; ok {
		versionStage = &value
	}
	output, err := s.authProvider.SecretManager().Manager.GetSecretValueWithContext(ctx, &secretsmanager.GetSecretValueInput{
		SecretId:     &req.Name,
		VersionId:    versionID,
		VersionStage: versionStage,
	})
	if err != nil {
		return secretstores.GetSecretResponse{Data: nil}, fmt.Errorf("couldn't get secret: %s", err)
	}

	resp := secretstores.GetSecretResponse{
		Data: map[string]string{},
	}
	if output.Name != nil && output.SecretString != nil {
		resp.Data[*output.Name] = *output.SecretString
	}

	return resp, nil
}

// BulkGetSecret retrieves all secrets in the store and returns a map of decrypted string/string values.
func (s *smSecretStore) BulkGetSecret(ctx context.Context, req secretstores.BulkGetSecretRequest) (secretstores.BulkGetSecretResponse, error) {
	resp := secretstores.BulkGetSecretResponse{
		Data: map[string]map[string]string{},
	}

	search := true
	var nextToken *string = nil

	for search {
		output, err := s.authProvider.SecretManager().Manager.ListSecretsWithContext(ctx, &secretsmanager.ListSecretsInput{
			MaxResults: nil,
			NextToken:  nextToken,
		})
		if err != nil {
			return secretstores.BulkGetSecretResponse{Data: nil}, fmt.Errorf("couldn't list secrets: %s", err)
		}

		for _, entry := range output.SecretList {
			secrets, err := s.authProvider.SecretManager().Manager.GetSecretValueWithContext(ctx, &secretsmanager.GetSecretValueInput{
				SecretId: entry.Name,
			})
			if err != nil {
				return secretstores.BulkGetSecretResponse{Data: nil}, fmt.Errorf("couldn't get secret: %s", *entry.Name)
			}

			if entry.Name != nil && secrets.SecretString != nil {
				resp.Data[*entry.Name] = map[string]string{*entry.Name: *secrets.SecretString}
			}
		}

		nextToken = output.NextToken
		search = output.NextToken != nil
	}

	return resp, nil
}

func (s *smSecretStore) getSecretManagerMetadata(spec secretstores.Metadata) (*SecretManagerMetaData, error) {
	b, err := json.Marshal(spec.Properties)
	if err != nil {
		return nil, err
	}

	var meta SecretManagerMetaData
	err = json.Unmarshal(b, &meta)
	if err != nil {
		return nil, err
	}

	return &meta, nil
}

// Features returns the features available in this secret store.
func (s *smSecretStore) Features() []secretstores.Feature {
	return []secretstores.Feature{} // No Feature supported.
}

func (s *smSecretStore) GetComponentMetadata() (metadataInfo metadata.MetadataMap) {
	metadataStruct := SecretManagerMetaData{}
	metadata.GetMetadataInfoFromStructType(reflect.TypeOf(metadataStruct), &metadataInfo, metadata.SecretStoreType)
	return
}

func (s *smSecretStore) Close() error {
	if s.authProvider != nil {
		return s.authProvider.Close()
	}
	return nil
}
