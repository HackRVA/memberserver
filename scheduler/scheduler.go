package scheduler

import (
	"io/ioutil"
	"memberserver/api/models"
	"memberserver/config"
	"memberserver/datastore"
	"memberserver/mail"
	"memberserver/payments"
	"memberserver/resourcemanager"
	"memberserver/resourcemanager/mqttserver"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// checkPaymentsInterval - check the resources every 24 hours
	checkPaymentsInterval = 24

	// evaluateMemberStatusInterval - check the resources every 25 hours
	evaluateMemberStatusInterval = 25

	// resourceStatusCheckInterval - check the resources every hour
	resourceStatusCheckInterval = 1

	resourceUpdateInterval = 4

	// checkIPInterval - check the IP Address daily
	checkIPInterval = 24
)

type Scheduler struct {
	config          config.Config
	dataStore       datastore.DataStore
	mailAPI         mail.MailApi
	resourceManager *resourcemanager.ResourceManager
	paymentProvider payments.PaymentProvider
}

type Task struct {
	interval time.Duration
	initFunc func()
	tickFunc func()
}

// Setup Scheduler
//  We want certain tasks to happen on a regular basis
//  The scheduler will make sure that happens
func (s *Scheduler) Setup(db datastore.DataStore) {
	s.config, _ = config.Load()
	s.mailAPI, _ = mail.Setup()
	s.dataStore = db
	s.resourceManager = resourcemanager.NewResourceManager(mqttserver.NewMQTTServer(), db)
	s.paymentProvider = payments.Setup(db)

	tasks := []Task{
		{interval: checkPaymentsInterval * time.Hour, initFunc: s.checkMemberSubscriptions, tickFunc: s.checkMemberSubscriptions},
		{interval: evaluateMemberStatusInterval * time.Hour, initFunc: s.resourceManager.RemovedInvalidUIDs, tickFunc: s.resourceManager.RemovedInvalidUIDs},
		{interval: evaluateMemberStatusInterval * time.Hour, initFunc: s.resourceManager.EnableValidUIDs, tickFunc: s.resourceManager.EnableValidUIDs},
		{interval: resourceStatusCheckInterval * time.Hour, initFunc: s.checkResourceInit, tickFunc: s.checkResourceTick},
		{interval: resourceUpdateInterval * time.Hour, initFunc: s.resourceManager.UpdateResources, tickFunc: s.resourceManager.UpdateResources},
		{interval: checkIPInterval * time.Hour, initFunc: s.checkIPAddressTick, tickFunc: s.checkIPAddressTick},
	}

	for _, task := range tasks {
		s.scheduleTask(task.interval, task.initFunc, task.tickFunc)
	}
}

func (s *Scheduler) scheduleTask(interval time.Duration, initFunc func(), tickFunc func()) {
	go initFunc()

	// quietly check the resource status on an interval
	ticker := time.NewTicker(interval)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				go tickFunc()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *Scheduler) checkMemberSubscriptions() {
	if len(s.config.PaypalURL) == 0 {
		log.Debugf("paypal url isn't set")
		return
	}
	members := s.dataStore.GetMembers()

	for _, member := range members {
		if member.Level == uint8(models.Credited) {
			// do nothing to credited members
			continue
		}
		if member.SubscriptionID == "none" || len(member.SubscriptionID) == 0 {
			// we might need to figure out why they don't have a subscriptionID
			log.Printf("no subscriptionID for: %s", member.Name)
			continue
		}

		status, value, err := s.paymentProvider.GetSubscription(member.SubscriptionID)
		if err != nil {
			log.Error(err)
			continue
		}
		if value == "" {
			log.Errorf("was not able to get payment amount: %s", value)
			continue
		}
		lastPayment, err := strconv.ParseFloat(value, 32)
		if err != nil {
			log.Error(err)
			continue
		}

		s.setMemberLevelBasedOnPaypalSubscriptionStatus(status, lastPayment, member)
	}
}

func (s *Scheduler) setMemberLevelBasedOnPaypalSubscriptionStatus(status string, lastPayment float64, member models.Member) {
	switch status {
	case payments.Active:
		if int64(lastPayment) == models.MemberLevelToAmount[models.Premium] {
			s.dataStore.SetMemberLevel(member.ID, models.Premium)
			return
		}
		if int64(lastPayment) == models.MemberLevelToAmount[models.Classic] {
			s.dataStore.SetMemberLevel(member.ID, models.Classic)
			return
		}
		s.dataStore.SetMemberLevel(member.ID, models.Standard)
	case payments.Canceled:
		s.dataStore.SetMemberLevel(member.ID, models.Inactive)
	case payments.Suspended:
		s.dataStore.SetMemberLevel(member.ID, models.Inactive)
	default:
		return
	}
}

func (s *Scheduler) checkResourceInit() {
	resources := s.dataStore.GetResources()

	// on startup we will subscribe to resources and publish an initial status check
	for _, r := range resources {
		s.resourceManager.MQTTServer.Subscribe(r.Name+"/send", resourcemanager.OnAccessEvent)
		s.resourceManager.MQTTServer.Subscribe(r.Name+"/result", resourcemanager.HealthCheck)
		s.resourceManager.MQTTServer.Subscribe(r.Name+"/sync", resourcemanager.OnHeartBeat)
		s.resourceManager.MQTTServer.Subscribe(r.Name+"/cleanup", resourcemanager.OnRemoveInvalidRequest)
		s.resourceManager.CheckStatus(r)
	}
}

func (s *Scheduler) checkResourceTick() {
	resources := s.dataStore.GetResources()

	for _, r := range resources {
		s.resourceManager.CheckStatus(r)
	}
}

var IPAddressCache string

func (s *Scheduler) checkIPAddressTick() {
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

	currentIp := strings.TrimSpace(string(body))
	log.Debugf("ip addr: %s", currentIp)

	const ipFileName string = ".public_ip_address"
	// detect if file exists
	_, err = os.Stat(ipFileName)

	// create file if not exists
	if os.IsNotExist(err) {
		var file, err = os.Create(ipFileName)
		if err != nil {
			log.Error(err)
			return
		}
		defer file.Close()
	}

	b, err := ioutil.ReadFile(ipFileName)
	if err != nil {
		log.Error(err)
		return
	}

	err = ioutil.WriteFile(ipFileName, body, 0644)
	if err != nil {
		log.Error(err)
		return
	}

	// if this is the first run, don't send an email,
	//   but set the ip address
	previousIp := strings.TrimSpace(string(b))
	if previousIp == "" || previousIp == currentIp {
		return
	}

	ipModel := struct {
		IpAddress string
	}{
		IpAddress: currentIp,
	}

	mailer := mail.NewMailer(s.dataStore, s.mailAPI, s.config)
	mailer.SendCommunication(mail.IpChanged, s.config.AdminEmail, ipModel)
}
