// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sessions

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test for GH-8 for CookieStore
func TestGH8CookieStore(t *testing.T) {
	originalPath := "/"
	store := NewCookieStore()
	store.Options.Path = originalPath
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	store.Options.Path = "/foo"
	if session.Options.Path != originalPath {
		t.Fatalf("bad session path: got %q, want %q", session.Options.Path, originalPath)
	}
}

// Test for GH-8 for FilesystemStore
func TestGH8FilesystemStore(t *testing.T) {
	originalPath := "/"
	store := NewFilesystemStore("")
	store.Options.Path = originalPath
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	store.Options.Path = "/foo"
	if session.Options.Path != originalPath {
		t.Fatalf("bad session path: got %q, want %q", session.Options.Path, originalPath)
	}
}

// Test for GH-2.
func TestGH2MaxLength(t *testing.T) {
	store := NewFilesystemStore("", []byte("some key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	w := httptest.NewRecorder()

	session, err := store.New(req, "my session")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	session.Values["big"] = make([]byte, base64.StdEncoding.DecodedLen(4096*2))
	err = session.Save(req, w)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	store.MaxLength(4096 * 3) // A bit more than the value size to account for encoding overhead.
	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to Save:", err)
	}
}

// Test delete filesystem store with max-age: -1
func TestGH8FilesystemStoreDelete(t *testing.T) {
	store := NewFilesystemStore("", []byte("some key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	w := httptest.NewRecorder()

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to save session", err)
	}

	session.Options.MaxAge = -1
	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to delete session", err)
	}
	filename := filepath.Join(store.path, sessionFilePrefix+filepath.Base(session.ID))
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		t.Fatalf("session file should be erased for max-age < 0, stat err: %v", err)
	}
}

// Test filesystem store with max-age: 0 keeps the session file and emits a
// browser-session cookie (no Max-Age attribute), per the Options contract.
func TestFilesystemStoreMaxAgeZeroKeepsFile(t *testing.T) {
	store := NewFilesystemStore("", []byte("some key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	w := httptest.NewRecorder()

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to save session", err)
	}
	filename := filepath.Join(store.path, sessionFilePrefix+filepath.Base(session.ID))
	if _, err := os.Stat(filename); err != nil {
		t.Fatalf("session file missing after initial save: %v", err)
	}

	session.Options.MaxAge = 0
	w = httptest.NewRecorder()
	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to save session with max-age 0", err)
	}
	if _, err := os.Stat(filename); err != nil {
		t.Fatalf("session file must survive max-age 0, got: %v", err)
	}
	cookie := w.Result().Cookies()[0]
	if cookie.MaxAge != 0 {
		t.Fatalf("expected browser-session cookie MaxAge 0, got %d", cookie.MaxAge)
	}
	if !cookie.Expires.IsZero() {
		t.Fatalf("expected no Expires attribute, got %v", cookie.Expires)
	}
}

// Test cookie store with max-age < 0 emits an expired, empty deletion cookie
// instead of re-encoding the session values.
func TestCookieStoreNegativeMaxAgeDeletesCredential(t *testing.T) {
	store := NewCookieStore([]byte("some key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	w := httptest.NewRecorder()

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}
	session.Values["user"] = "super-secret-login"
	session.Options.MaxAge = -1

	if err := session.Save(req, w); err != nil {
		t.Fatal("failed to delete session", err)
	}
	cookie := w.Result().Cookies()[0]
	if cookie.Value != "" {
		t.Fatalf("deletion cookie must not carry session values, got %q", cookie.Value)
	}
	if cookie.MaxAge != -1 {
		t.Fatalf("expected MaxAge -1, got %d", cookie.MaxAge)
	}
	if !cookie.Expires.Equal(time.Unix(1, 0)) {
		t.Fatalf("expected epoch expiry, got %v", cookie.Expires)
	}
}
