/*
 * Copyright (C) 2014-2015 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package snappy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubuntu-core/snappy/snappy"
	"launchpad.net/webdm/webprogress"

	"github.com/gorilla/mux"
)

// Handler implements snappy's packages api.
type Handler struct {
	statusTracker *webprogress.StatusTracker
}

// NewHandler creates an instance that implements snappy's packages api.
func NewHandler() *Handler {
	return &Handler{
		statusTracker: webprogress.New(),
	}
}

func installedOnly(v string) bool {
	return v == "true"
}

func types(v string) []string {
	return strings.Split(v, ",")
}

func (h *Handler) getAll(w http.ResponseWriter, r *http.Request) {
	if e := SimpleCookieCheckOrRedirect(w, r); e != nil {
		return
	}

	log.Println(r.Method, r.URL.Path)

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)

	filter := listFilter{
		installedOnly: installedOnly(r.FormValue("installed_only")),
		types:         types(r.FormValue("types")),
		query:         r.FormValue("q"),
	}

	payload, err := h.allPackages(&filter)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		enc.Encode(fmt.Sprintf("Error: %s", err))
		return
	}

	if err := enc.Encode(payload); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		// give up on json
		fmt.Fprintf(w, "Error: %s", err)
		log.Print(err)
	}
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if e := SimpleCookieCheckOrRedirect(w, r); e != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Get the Key.
	vars := mux.Vars(r)
	resource := vars["id"]
	enc := json.NewEncoder(w)

	payload, err := h.packagePayload(resource)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		enc.Encode(fmt.Sprintln(err, resource))
		return
	}

	if payload.IsError {
		w.WriteHeader(http.StatusInternalServerError)
	}

	if err := enc.Encode(payload); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		// give up on json
		fmt.Fprintf(w, "Error: %s", err)
		log.Print(err)
	}
}

func (h *Handler) add(w http.ResponseWriter, r *http.Request) {
	if e := SimpleCookieCheckOrRedirect(w, r); e != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Get the Key.
	vars := mux.Vars(r)
	ID := vars["id"]

	err := h.installPackage(ID)
	msg, status := respond(err)

	response := response{Message: msg, Package: ID}
	bs, err := json.Marshal(response)
	if err != nil {
		// giving up on json
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error: %s", err)
		log.Print(err)
		return
	}

	w.WriteHeader(status)
	w.Write(bs)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	if e := SimpleCookieCheckOrRedirect(w, r); e != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Get the Key.
	vars := mux.Vars(r)
	ID := vars["id"]

	err := h.removePackage(ID)
	msg, status := respond(err)

	response := response{Message: msg, Package: ID}
	bs, err := json.Marshal(response)
	if err != nil {
		// giving up on json
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error: %s", err)
		log.Print(err)
		return
	}

	w.WriteHeader(status)
	w.Write(bs)
}

func respond(err error) (msg string, status int) {
	switch err {
	case snappy.ErrAlreadyInstalled:
		status = http.StatusOK
		msg = "Installed"
	case webprogress.ErrPackageInstallInProgress:
		status = http.StatusBadRequest
		msg = "Operation in progress"
	case snappy.ErrPackageNotFound:
		status = http.StatusNotFound
		msg = "Package not found"
	case nil:
		status = http.StatusAccepted
		msg = "Accepted"
	default:
		status = http.StatusInternalServerError
		msg = "Processing error"
	}

	return msg, status
}

// MakeMuxer sets up the handlers multiplexing to handle requests against snappy's
// packages api
func (h *Handler) MakeMuxer(prefix string) http.Handler {
	var m *mux.Router

	if prefix == "" {
		m = mux.NewRouter()
	} else {
		m = mux.NewRouter().PathPrefix(prefix).Subrouter()
	}

	// Get all of packages.
	m.HandleFunc("/", h.getAll).Methods("GET")

	// get specific package
	m.HandleFunc("/{id}", h.get).Methods("GET")

	// Add a package
	m.HandleFunc("/{id}", h.add).Methods("PUT")

	// Remove a package
	m.HandleFunc("/{id}", h.remove).Methods("DELETE")

	return m
}

// TODO: refactor this copy from cmd/snapweb

// Name of the cookie transporting the access token
const (
	SnapwebCookieName = "SM"
)

func tokenFilename() string {
	return filepath.Join(os.Getenv("SNAP_APP_PATH"), "token.txt")
}

// SimpleCookieCheckOrRedirect is a simple authorization mechanism
func SimpleCookieCheckOrRedirect(w http.ResponseWriter, r *http.Request) error {
	cookie, _ := r.Cookie(SnapwebCookieName)
	if cookie != nil {
		token, err := ioutil.ReadFile(tokenFilename())
		if err == nil {
			if string(token) == cookie.Value {
				// the auth-token and the cookie do match
				// we can continue with the request
				return nil
			}
		}
	}

	// in any other case, refuse the request and redirect
	http.Redirect(w, r, "/access-control", 401)

	return errors.New("Unauthorized")
}
