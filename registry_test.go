// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sessions

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRegistryGetNewSameInstance ensures that Get() and New() resolve to the
// same registered instance for a given name during a request, so writes made
// through either entry point are persisted by Registry.Save().
func TestRegistryGetNewSameInstance(t *testing.T) {
	store := NewCookieStore([]byte("secret-key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}

	got, err := store.Get(req, "session-key")
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}

	created, err := store.New(req, "session-key")
	if err != nil {
		t.Fatalf("Error creating session: %v", err)
	}
	if got != created {
		t.Fatal("Get() and New() returned different session instances for the same name")
	}

	// The second middleware writes through the instance returned by New().
	created.Values["from-new"] = "value"

	rsp := httptest.NewRecorder()
	if err := Save(req, rsp); err != nil {
		t.Fatalf("Error saving session: %v", err)
	}

	// Decode the persisted cookie in a follow-up request and verify the write
	// made via New() was not silently dropped.
	req2, _ := http.NewRequest("GET", "http://www.example.com", nil)
	req2.Header.Add("Cookie", rsp.Header().Get("Set-Cookie"))
	session, err := store.Get(req2, "session-key")
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}
	if session.Values["from-new"] != "value" {
		t.Fatalf("write through New() was not persisted, got %v", session.Values)
	}
}

// TestRegistryNewRetriesDecodeError covers the cached decode error: within a
// request the error is returned by both Get() and New() on the single
// registered instance (no unregistered duplicate), while the following
// request re-runs the decode once the cookie has been fixed.
func TestRegistryNewRetriesDecodeError(t *testing.T) {
	store := NewCookieStore([]byte("secret-key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	req.Header.Set("Cookie", "session-key=invalid-cookie-value")

	// First decode fails and the failure is cached alongside the session.
	session, err := store.Get(req, "session-key")
	if err == nil {
		t.Fatal("expected a decode error for the invalid cookie, got nil")
	}

	// Within the same request, New() reuses the registered instance and keeps
	// returning the cached decode error instead of building a second,
	// unregistered session.
	again, err := store.New(req, "session-key")
	if err == nil {
		t.Fatal("expected the cached decode error to remain for the request")
	}
	if again != session {
		t.Fatal("New() returned a different instance than Get()")
	}

	// A subsequent request with a valid cookie decodes successfully.
	rsp := httptest.NewRecorder()
	freshReq, _ := http.NewRequest("GET", "http://www.example.com", nil)
	good, err := store.New(freshReq, "session-key")
	if err != nil {
		t.Fatalf("Error creating session: %v", err)
	}
	good.Values["ok"] = true
	if err := good.Save(freshReq, rsp); err != nil {
		t.Fatalf("Error saving session: %v", err)
	}

	retryReq, _ := http.NewRequest("GET", "http://www.example.com", nil)
	retryReq.Header.Add("Cookie", rsp.Header().Get("Set-Cookie"))
	retried, err := store.New(retryReq, "session-key")
	if err != nil {
		t.Fatalf("retry after fixing the cookie failed: %v", err)
	}
	if retried.Values["ok"] != true {
		t.Fatalf("retry did not re-decode the fixed cookie, got %v", retried.Values)
	}
}
