package resourcemanager

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"

	"memberserver/database"
)

// Resource manager keeps the resources up to date by
//  pushing new updates and checking in on their health

// ResourceManager contains functions that
type ResourceManager struct {
	db *database.Database
}

// ACLUpdateRequest is the json object we send to a resource when pushing an update
type ACLUpdateRequest struct {
	ACL []string `json:"acl"`
}

// Setup initializes the resource manager
func Setup() (*ResourceManager, error) {
	var err error
	rm := &ResourceManager{}
	rm.db, err = database.Setup()

	if err != nil {
		log.Errorf("error setting up db: %s", err)
		return rm, err
	}

	return rm, err
}

// UpdateResourceACL pulls a resource's accesslist from the DB and pushes it to the resource
func (rm *ResourceManager) UpdateResourceACL(r database.Resource) error {
	// get acl for that resource
	accessList, err := rm.db.GetResourceACL(r)

	if err != nil {
		return err
	}

	updateRequest := &ACLUpdateRequest{}
	updateRequest.ACL = accessList

	j, err := json.Marshal(updateRequest)
	if err != nil {
		return err
	}

	// push the update to the resource
	resp, err := http.Post(r.Address+"/update", "application/json", bytes.NewBuffer(j))
	if err != nil {
		log.Errorf("Unable to reach the resource.")
		return err
	} else {
		body, _ := ioutil.ReadAll(resp.Body)

		// TODO: check that the resource responds with a hash of the list
		log.Debugf("body=", string(body))
	}

	return nil
}

// ACLResponse Response from a resource that is a hash of the ACL that the
//   resource has stored
type ACLResponse struct {
	ACLHash string `json:"acl"`
}

// CheckStatus will make an http request to verify that the resource has the correct
//   and up to date access list
//   It will do this by hashing the list retrieved from the DB and comparing it
//   with the hash that the resource reports
func (rm *ResourceManager) CheckStatus(r database.Resource) error {
	accessList, err := rm.db.GetResourceACL(r)

	if err != nil {
		return err
	}

	// TODO hash the accesslist
	println(hash(accessList))

	// push the update to the resource
	resp, err := http.Get(r.Address)
	if err != nil {
		log.Errorf("Unable to reach the resource.")
		return err
	} else {
		body, _ := ioutil.ReadAll(resp.Body)

		// TODO: check that the resource responds with a hash of the list
		log.Debugf("body=", string(body))
	}
	return nil
}

func hash(accessList []string) string {
	h := sha1.New()
	h.Write([]byte(strings.Join(accessList[:], "\n")))
	bs := h.Sum(nil)

	log.Debug(strings.Join(accessList[:], "\n"))
	log.Debugf("%x\n", bs)
	return fmt.Sprintf("%x\n", bs)
}
