package scheduler

import (
	"io/ioutil"
	"memberserver/database"
	"memberserver/mail"
	"memberserver/payments"
	"memberserver/resourcemanager"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// checkPaymentsInterval - check the resources every 24 hours
const checkPaymentsInterval = 24

// evaluateMemberStatusInterval - check the resources every 25 hours
const evaluateMemberStatusInterval = 25

// resourceStatusCheckInterval - check the resources every hour
const resourceStatusCheckInterval = 1

const resourceUpdateInterval = 4

// checkIPInterval - check the IP Address daily
const checkIPInterval = 24

// Setup Scheduler
//  We want certain tasks to happen on a regular basis
//  The scheduler will make sure that happens
func Setup() {
	scheduleTask(checkPaymentsInterval*time.Hour, payments.GetPayments, payments.GetPayments)
	scheduleTask(evaluateMemberStatusInterval*time.Hour, checkMemberStatus, checkMemberStatus)
	scheduleTask(resourceStatusCheckInterval*time.Hour, checkResourceInit, checkResourceTick)
	scheduleTask(resourceUpdateInterval*time.Hour, resourcemanager.UpdateResources, resourcemanager.UpdateResources)
	scheduleTask(checkIPInterval*time.Hour, checkIPAddressTick, checkIPAddressTick)
}

func scheduleTask(interval time.Duration, initFunc func(), tickFunc func()) {
	initFunc()

	// quietly check the resource status on an interval
	ticker := time.NewTicker(interval)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				tickFunc()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
}

func checkMemberStatus() {
	db, err := database.Setup()
	if err != nil {
		log.Errorf("error setting up db: %s", err)
	}

	members := db.GetMembers()
	defer db.Release()

	for _, m := range members {
		err = db.EvaluateMemberStatus(m.ID)
		if err != nil {
			log.Errorf("error evaluating member's status: %s", err.Error())
		}
	}
}

func checkResourceInit() {
	db, err := database.Setup()
	if err != nil {
		log.Errorf("error setting up db: %s", err)
		return
	}

	resources := db.GetResources()
	defer db.Release()

	// on startup we will subscribe to resources and publish an initial status check
	for _, r := range resources {
		resourcemanager.Subscribe(r.Name+"/send", resourcemanager.OnAccessEvent)
		resourcemanager.Subscribe(r.Name+"/result", resourcemanager.HealthCheck)
		resourcemanager.Subscribe(r.Name+"/sync", resourcemanager.OnHeartBeat)
		resourcemanager.CheckStatus(r)
	}
}

func checkResourceTick() {
	db, err := database.Setup()
	if err != nil {
		log.Errorf("error setting up db: %s", err)
		return
	}

	resources := db.GetResources()
	defer db.Release()

	for _, r := range resources {
		resourcemanager.CheckStatus(r)
	}
}

var IPAddressCache string

func checkIPAddressTick() {
	resp, err := http.Get("https://icanhazip.com/")
	if err != nil {
		log.Errorf("can't get IP address: %s", err)
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return
	}

	log.Debugf("ip addr: %s", string(body))

	// if this is the first run, don't send an email,
	//   but set the ip address
	if IPAddressCache == "" {
		IPAddressCache = string(body)
		return
	}

	IPAddressCache = string(body)
	mail.SendIPHasChanged()
}
