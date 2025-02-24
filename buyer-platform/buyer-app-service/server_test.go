// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/pubsub"
	"github.com/google/go-cmp/cmp"

	"ondc/shared/config"
	"ondc/shared/models/model"
	"ondc/shared/pubsubtest"

	_ "embed"
)

var (
	//go:embed testdata/ack_response.json
	ackResponsePayload []byte

	//go:embed testdata/search_request.json
	searchRequestPayload []byte
	//go:embed testdata/select_request.json
	selectRequestPayload []byte
	//go:embed testdata/init_request.json
	initRequestPayload []byte
	//go:embed testdata/confirm_request.json
	confirmRequestPayload []byte
	//go:embed testdata/status_request.json
	statusRequestPayload []byte
	//go:embed testdata/track_request.json
	trackRequestPayload []byte
	//go:embed testdata/cancel_request.json
	cancelRequestPayload []byte
	//go:embed testdata/update_request.json
	updateRequestPayload []byte
	//go:embed testdata/rating_request.json
	ratingRequestPayload []byte
	//go:embed testdata/support_request.json
	supportRequestPayload []byte

	//go:embed testdata/invalid_request.json
	invalidRequestPayload []byte
)

func TestInitServerSuccess(t *testing.T) {
	const (
		projectID  = "test-project"
		topicID    = "test-topic"
		instanceID = "test-instance"
		databaseID = "test-database"
	)
	ctx := context.Background()
	conf := config.BuyerAppConfig{
		ProjectID: projectID,
		TopicID:   topicID,
	}

	psSetups := []pubsubtest.PubsubSetup{
		{TopicID: topicID},
	}
	_, opt := pubsubtest.InitServer(t, projectID, psSetups)
	pubsubClient, err := pubsub.NewClient(ctx, conf.ProjectID, opt)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if _, err := initServer(ctx, conf, pubsubClient); err != nil {
		t.Errorf("intializeServer() failed: %v", err)
	}
}

func TestHandlersSuccess(t *testing.T) {
	const (
		projectID  = "test-project"
		topicID    = "test-topic"
		instanceID = "test-instance"
		databaseID = "test-database"
	)
	ctx := context.Background()
	conf := config.BuyerAppConfig{
		ProjectID: projectID,
		TopicID:   topicID,
	}

	psSetups := []pubsubtest.PubsubSetup{
		{TopicID: topicID},
	}
	psSrv, opt := pubsubtest.InitServer(t, projectID, psSetups)
	pubsubClient, err := pubsub.NewClient(ctx, conf.ProjectID, opt)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	srv, err := initServer(ctx, conf, pubsubClient)
	if err != nil {
		t.Errorf("intializeServer() failed: %v", err)
	}

	var wantAck model.AckResponse
	if err := json.Unmarshal(ackResponsePayload, &wantAck); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	tests := [10]struct {
		handlerName string
		handler     http.HandlerFunc
		path        string
		body        []byte
	}{
		{
			handlerName: "searchHandler",
			handler:     srv.searchHandler,
			path:        "/search",
			body:        searchRequestPayload,
		},
		{
			handlerName: "selectHandler",
			handler:     srv.selectHandler,
			path:        "/select",
			body:        selectRequestPayload,
		},
		{
			handlerName: "initHandler",
			handler:     srv.initHandler,
			path:        "/init",
			body:        initRequestPayload,
		},
		{
			handlerName: "confirmHandler",
			handler:     srv.confirmHandler,
			path:        "/confirm",
			body:        confirmRequestPayload,
		},
		{
			handlerName: "statusHandler",
			handler:     srv.statusHandler,
			path:        "/status",
			body:        statusRequestPayload,
		},
		{
			handlerName: "trackHandler",
			handler:     srv.trackHandler,
			path:        "/track",
			body:        trackRequestPayload,
		},
		{
			handlerName: "cancelHandler",
			handler:     srv.cancelHandler,
			path:        "/cancel",
			body:        cancelRequestPayload,
		},
		{
			handlerName: "updateHandler",
			handler:     srv.updateHandler,
			path:        "/update",
			body:        updateRequestPayload,
		},
		{
			handlerName: "ratingHandler",
			handler:     srv.ratingHandler,
			path:        "/rating",
			body:        ratingRequestPayload,
		},
		{
			handlerName: "supportHandler",
			handler:     srv.supportHandler,
			path:        "/support",
			body:        supportRequestPayload,
		},
	}
	for _, test := range tests {
		test := test // Make a local copy of test data for safety.
		t.Run(test.handlerName, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewReader(test.body))
			response := httptest.NewRecorder()

			test.handler(response, request)

			if got, want := response.Code, http.StatusOK; got != want {
				t.Errorf("%s got status %d, want %d", test.handlerName, got, want)
				t.Logf("Response body: %s", response.Body.Bytes())
			}

			var gotAck model.AckResponse
			if err := json.Unmarshal(response.Body.Bytes(), &gotAck); err != nil {
				t.Fatalf("%s Unmarshal response body got error: %v", test.handlerName, err)
			}
			if diff := cmp.Diff(wantAck, gotAck); diff != "" {
				t.Errorf("%s response body diff (-want, +got):\n%s", test.handlerName, diff)
			}

			msgID := response.Header().Get(psMsgIDHeader)
			psMsg := psSrv.Message(msgID)
			if psMsg == nil {
				t.Fatalf("%s publish no message", test.handlerName)
			}
			if bytes.Compare(psMsg.Data, test.body) != 0 {
				t.Errorf("%s Pub/Sub message data is not equal to request body", test.handlerName)
			}
		})
	}
}

func TestHandlersInvalidPayload(t *testing.T) {
	const (
		projectID  = "test-project"
		topicID    = "test-topic"
		instanceID = "test-instance"
		databaseID = "test-database"
	)
	ctx := context.Background()
	conf := config.BuyerAppConfig{
		ProjectID: projectID,
		TopicID:   topicID,
	}

	psSetups := []pubsubtest.PubsubSetup{
		{TopicID: topicID},
	}
	psSrv, opt := pubsubtest.InitServer(t, projectID, psSetups)
	pubsubClient, err := pubsub.NewClient(ctx, conf.ProjectID, opt)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	srv, err := initServer(ctx, conf, pubsubClient)
	if err != nil {
		t.Errorf("intializeServer() failed: %v", err)
	}

	wantErroCode := "30000"
	wantAck := model.AckResponse{
		Message: &model.MessageAck{
			Ack: &model.Ack{
				Status: "NACK",
			},
		},
		Error: &model.Error{
			Type: "JSON-SCHEMA-ERROR",
			Code: &wantErroCode,
		},
	}

	tests := [10]struct {
		handlerName string
		handler     http.HandlerFunc
		path        string
	}{
		{
			handlerName: "searchHandler",
			handler:     srv.searchHandler,
			path:        "/search",
		},
		{
			handlerName: "selectHandler",
			handler:     srv.selectHandler,
			path:        "/select",
		},
		{
			handlerName: "initHandler",
			handler:     srv.initHandler,
			path:        "/init",
		},
		{
			handlerName: "confirmHandler",
			handler:     srv.confirmHandler,
			path:        "/confirm",
		},
		{
			handlerName: "statusHandler",
			handler:     srv.statusHandler,
			path:        "/status",
		},
		{
			handlerName: "trackHandler",
			handler:     srv.trackHandler,
			path:        "/track",
		},
		{
			handlerName: "cancelHandler",
			handler:     srv.cancelHandler,
			path:        "/cancel",
		},
		{
			handlerName: "updateHandler",
			handler:     srv.updateHandler,
			path:        "/update",
		},
		{
			handlerName: "ratingHandler",
			handler:     srv.ratingHandler,
			path:        "/rating",
		},
		{
			handlerName: "supportHandler",
			handler:     srv.supportHandler,
			path:        "/support",
		},
	}
	for _, test := range tests {
		test := test // Make a local copy of test data for safety.
		t.Run(test.handlerName, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewReader(invalidRequestPayload))
			response := httptest.NewRecorder()

			test.handler(response, request)

			if got, want := response.Code, http.StatusBadRequest; got != want {
				t.Errorf("%s got status %d, want %d", test.handlerName, got, want)
				t.Logf("Response body: %s", response.Body.Bytes())
			}

			var gotAck model.AckResponse
			if err := json.Unmarshal(response.Body.Bytes(), &gotAck); err != nil {
				t.Fatalf("%s Unmarshal response body got error: %v", test.handlerName, err)
			}
			if diff := cmp.Diff(wantAck, gotAck); diff != "" {
				t.Errorf("%s response body diff (-want, +got):\n%s", test.handlerName, diff)
			}

			msgID := response.Header().Get(psMsgIDHeader)
			psMsg := psSrv.Message(msgID)
			if psMsg != nil {
				t.Errorf("%s unexpectedly publish a message", test.handlerName)
			}
		})
	}
}
