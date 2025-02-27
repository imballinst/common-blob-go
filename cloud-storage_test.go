/*
 * Copyright (c) 2020 AccelByte Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and limitations under the License.
 *
 */

package commonblobgo

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

func TestAWSAPISuite(t *testing.T) {
	suite.Run(t, &Suite{
		isTesting:      true,
		bucketName:     "gdpr-req-data",
		bucketProvider: "aws",

		awsS3Endpoint:        "http://localhost:4572",
		awsS3Region:          "us-west-2",
		awsS3AccessKeyID:     "AWS_ACCESS_KEY_ID",
		awsS3SecretAccessKey: "AWS_SECRET_ACCESS_KEY",
	})
}

func TestGCPAPISuite(t *testing.T) {
	suite.Run(t, &Suite{
		isTesting:              true,
		bucketName:             "gdpr-req-data",
		bucketProvider:         "gcp",
		gcpCredentialsJSON:     `{"type": "service_account", "project_id": "my-project-id"}`,
		gcpStorageEmulatorHost: "0.0.0.0:4443",
	})
}

func TestAWSDemoAPISuite(t *testing.T) {
	// warning, this suite uses real S3 credentials
	awsS3Endpoint := os.Getenv("AWS_S3_ENDPOINT")
	awsS3Region := os.Getenv("AWS_REGION")
	awsS3AccessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	awsS3SecretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if awsS3Region == "" {
		t.Skipf("Skipped. Required ENV variable AWS_REGION")
		return
	}

	if awsS3AccessKeyID == "" {
		t.Skipf("Skipped. Required ENV variable AWS_ACCESS_KEY_ID")
		return
	}

	if awsS3SecretAccessKey == "" {
		t.Skipf("Skipped. Required ENV variable AWS_SECRET_ACCESS_KEY")
		return
	}

	suite.Run(t, &Suite{
		isTesting:      false,
		bucketName:     "gdpr-req-data",
		bucketProvider: "aws",

		awsS3Endpoint:        awsS3Endpoint,
		awsS3Region:          awsS3Region,
		awsS3AccessKeyID:     awsS3AccessKeyID,
		awsS3SecretAccessKey: awsS3SecretAccessKey,
	})
}

func TestGCPDemoAPISuite(t *testing.T) {
	// warning, this suite uses real GCP credentials
	gcpCredentialsJSON := os.Getenv("GCP_CREDENTIAL_JSON")

	if gcpCredentialsJSON == "" {
		t.Skipf("Skipped. Required ENV variable GCP_CREDENTIAL_JSON")
		return
	}

	suite.Run(t, &Suite{
		isTesting:          false,
		bucketName:         "gdpr-req-data",
		bucketProvider:     "gcp",
		gcpCredentialsJSON: gcpCredentialsJSON,
	})
}

type Suite struct {
	suite.Suite

	storage CloudStorage

	ctx            context.Context
	isTesting      bool
	bucketProvider string
	bucketName     string
	bucketPrefix   string

	awsS3Endpoint        string
	awsS3Region          string
	awsS3AccessKeyID     string
	awsS3SecretAccessKey string

	gcpCredentialsJSON     string
	gcpStorageEmulatorHost string // only for tests
}

func (s *Suite) SetupSuite() {
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetReportCaller(true)

	s.ctx = context.Background()
	s.bucketPrefix = fmt.Sprintf("test_%s", uuid.New().String())

	storage, err := NewCloudStorage(
		s.ctx,
		s.isTesting,
		s.bucketProvider,
		s.bucketName,
		s.awsS3Endpoint,
		s.awsS3Region,
		s.awsS3AccessKeyID,
		s.awsS3SecretAccessKey,
		s.gcpCredentialsJSON,
		s.gcpStorageEmulatorHost,
	)
	s.Require().NoError(err)
	s.Require().NotNil(storage)

	s.storage = storage

	err = s.storage.CreateBucket(s.ctx, s.bucketPrefix, 1)
	s.Require().NoError(err)
}

func (s *Suite) generateFileName() string {
	return fmt.Sprintf("%s/%s.json", s.bucketPrefix, uuid.New().String())
}

func (s *Suite) TestCreateBucket() {
	prefix := uuid.New().String()

	err := s.storage.CreateBucket(s.ctx, prefix, 1)
	s.Require().NoError(err)
}

func (s *Suite) TestWriteAndGet() {
	fileName := s.generateFileName()
	body := []byte(`{"key": "value"}`)

	err := s.storage.Write(s.ctx, fileName, body, nil)
	s.Require().NoError(err)

	storedBody, err := s.storage.Get(s.ctx, fileName)
	s.Require().NoError(err)
	s.Require().NotEmpty(storedBody)

	s.Require().JSONEq(string(body), string(storedBody))
}

func (s *Suite) TestWriteAndGetUsingReaderAndWriter() {
	fileName := s.generateFileName()
	body := []byte(`{"key": "value", "key2": "value2"}`)

	writer, err := s.storage.GetWriter(s.ctx, fileName)
	s.Require().NoError(err)

	_, err = writer.Write(body[:10])
	s.Require().NoError(err)

	_, err = writer.Write(body[10:20])
	s.Require().NoError(err)

	_, err = writer.Write(body[20:])
	s.Require().NoError(err)

	err = writer.Close()
	s.Require().NoError(err)

	reader, err := s.storage.GetReader(s.ctx, fileName)
	s.Require().NoError(err)

	storedBody, err := ioutil.ReadAll(reader)
	s.Require().NoError(err)
	s.Require().NotEmpty(storedBody)

	err = reader.Close()
	s.Require().NoError(err)

	s.Require().JSONEq(string(body), string(storedBody))
}

func (s *Suite) TestWriteAndGetUsingRangeReader() {
	fileName := s.generateFileName()
	body := []byte(`0123456789`)

	writer, err := s.storage.GetWriter(s.ctx, fileName)
	s.Require().NoError(err)

	_, err = writer.Write(body[:10])
	s.Require().NoError(err)

	err = writer.Close()
	s.Require().NoError(err)

	// Read chunk 1 : offset:0, length:5
	rangeReader, err := s.storage.GetRangeReader(s.ctx, fileName, 0, 5)
	s.Require().NoError(err)

	chunk1Result, err := ioutil.ReadAll(rangeReader)
	s.Require().NoError(err)
	s.Require().NotEmpty(chunk1Result)

	err = rangeReader.Close()

	s.Require().NoError(err)
	s.Require().Equal(string(chunk1Result), "01234")

	// Read chunk 2 : offset:5, length:5
	rangeReader, err = s.storage.GetRangeReader(s.ctx, fileName, 5, 5)
	s.Require().NoError(err)

	chunk2Result, err := ioutil.ReadAll(rangeReader)
	s.Require().NoError(err)
	s.Require().NotEmpty(chunk2Result)

	err = rangeReader.Close()

	s.Require().NoError(err)
	s.Require().Equal(string(chunk2Result), "56789")
}

func (s *Suite) TestWriteAndList() {
	fileName := s.generateFileName()
	body := []byte(`{"key": "value"}`)

	err := s.storage.Write(s.ctx, fileName, body, nil)
	s.Require().NoError(err)

	var fileFound bool

	list := s.storage.List(s.ctx, s.bucketPrefix)

	for {
		item, err := list.Next(s.ctx)
		if err == io.EOF {
			break
		}

		s.Require().NoError(err)

		if item.Key == fileName {
			fileFound = true
		}
	}

	s.Require().True(fileFound)
}

func (s *Suite) TestAttributes() {
	fileName := s.generateFileName()
	body := []byte(`{"key": "value"}`)

	err := s.storage.Write(s.ctx, fileName, body, nil)
	s.Require().NoError(err)

	attrs, err := s.storage.Attributes(s.ctx, fileName)
	s.Require().NoError(err)
	s.Require().Equal(int64(len(body)), attrs.Size)
	s.Require().True(attrs.ModTime.Before(time.Now()))
}

func (s *Suite) TestDelete() {
	fileName := s.generateFileName()
	body := []byte(`{"key": "value"}`)

	err := s.storage.Write(s.ctx, fileName, body, nil)
	s.Require().NoError(err)

	storedBody, err := s.storage.Get(s.ctx, fileName)
	s.Require().NoError(err)
	s.Require().NotEmpty(storedBody)

	err = s.storage.Delete(s.ctx, fileName)
	s.Require().NoError(err)

	_, err = s.storage.Get(s.ctx, fileName)
	s.Require().Error(err)
}

func (s *Suite) TestGetSignedURL() {
	fileName := s.generateFileName()
	body := []byte(`{"key": "value"}`)

	err := s.storage.Write(s.ctx, fileName, body, nil)
	s.Require().NoError(err)

	storedBody, err := s.storage.Get(s.ctx, fileName)
	s.Require().NoError(err)
	s.Require().NotEmpty(storedBody)

	options := &SignedURLOption{
		Expiry:                   time.Hour,
		Method:                   "GET",
		ContentType:              "",
		EnforceAbsentContentType: false,
	}

	url, err := s.storage.GetSignedURL(s.ctx, fileName, options)
	s.Require().NoError(err)
	s.Require().NotEmpty(url)
}
