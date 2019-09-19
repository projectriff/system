/*
Copyright 2019 the original author or authors.

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

package streaming

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"

	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
)

type StreamProvisionerClient interface {
	ProvisionStream(stream *streamingv1alpha1.Stream) (*streamingv1alpha1.StreamAddress, error)
}

type streamProvisionerRestClient struct {
	httpClient *http.Client
	logger     logr.Logger
}

func NewStreamProvisionerClient(httpClient *http.Client, logger logr.Logger) StreamProvisionerClient {
	return &streamProvisionerRestClient{
		httpClient: httpClient,
		logger:     logger,
	}
}

func (s *streamProvisionerRestClient) ProvisionStream(stream *streamingv1alpha1.Stream) (*streamingv1alpha1.StreamAddress, error) {
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local/%s/%s", stream.Spec.Provider, stream.Namespace, stream.Namespace, stream.Name)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader([]byte{}))
	if err != nil {
		return nil, err
	}
	//req.Header.Add("content-type", "application/json")
	res, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			s.logger.Error(err, "Error closing stream creation response body")
		}
	}()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("status: %d", res.StatusCode)
	}
	address := &streamingv1alpha1.StreamAddress{}
	if err := json.NewDecoder(res.Body).Decode(address); err != nil {
		return nil, err
	}
	return address, nil
}
