package main

import (
	"crypto/rsa"
	"encoding/json"
	"net/http"

	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/log"

	"github.com/gorilla/mux"
	"github.com/volcanicpixels/licensing/license"
)

type appHandler func(context.Context, http.ResponseWriter, *http.Request) *appError

type appError struct {
	Error   error
	Message string
	Code    int
}

func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if e := fn(c, w, r); e != nil {
		log.Errorf(c, "[%v] %v", e.Message, e.Error)
		http.Error(w, e.Message, e.Code)
	}
}

// NewLicense handles POST requests on /api/licenses/create
//
// The request body must contain a JSON object with a product field
//
// Examples:
//
//  POST /api/licenses {"product": ""}
//  400 empty title
//
//  POST /api/licenses {"product": "domain_changer"}
//  200
func NewLicense(c context.Context, w http.ResponseWriter, r *http.Request) *appError {
	var req struct{ Product string }
	var err error

	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		return &appError{err, "Could not decode json request", http.StatusBadRequest}
	}

	var key *rsa.PrivateKey
	if key, err = getPrivateKey(c, "plugin"); err != nil {
		return &appError{err, "Could not load private key for signing", http.StatusInternalServerError}
	}

	// create the license
	lic := license.New(req.Product)

	var licStr string
	if licStr, err = lic.Encode(key); err != nil {
		return &appError{err, "Could not encode the license", http.StatusInternalServerError}
	}

	writeJSON(w, 200, licStr)
	return nil
}

func revokeLicense(c context.Context, id string) error {
	// ideally we would simply add the license ID on to the end of the revocations.txt file
	// but Google Storage doesn't support appends.
	// It does support a composition operation, so we could write the new ID to a new file
	// and then compose the original with the new one to ensure atomicity, except the Google
	// storage client library does not implement this operation.
	// Therefore the best we can do without stupidly complex locks is to simply read in the current file
	// and then write a new file with the addition

	// read the current revocations.txt file
	sc := NewStorageContext(c)
	data, err := sc.ReadFile("revocations.txt")

	if err != nil {
		return err
	}

	line := id

	// almost certainly a better way to do this
	data = []byte(string(data) + "\n" + line)

	if err := sc.WriteFile("revocations.txt", data); err != nil {
		return err
	}

	return nil
}

// RevokeLicense handles POST requests to /api/licenses/{ID}/revoke
func RevokeLicense(c context.Context, w http.ResponseWriter, r *http.Request) *appError {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := revokeLicense(c, id); err != nil {
		return &appError{err, "An error occurred updating the revocations file", http.StatusInternalServerError}
	}

	writeJSON(w, 200, "SUCCESS")

	return nil
}
