package controllers

import (
	"memberserver/internal/models"
	"memberserver/pkg/paypal/listener"

	log "github.com/sirupsen/logrus"
)

// PaypalSubscriptionWebHookHandler paypal will tell us when a new subscription is created.
//   We can use this to add a member to our database.  We don't have to give them
//   access to anything at this time, but it will make it easier to assign them an RFID fob
func (api API) PaypalSubscriptionWebHookHandler(err error, n *listener.Subscription) {
	if err != nil {
		log.Printf("IPN error: %v", err)
		return
	}

	log.Printf("event type: %s", n.EventType)
	log.Printf("event resource type: %s", n.ResourceType)
	log.Printf("summary: %s", n.Summary)
	log.Printf("name: %s", n.Resource.Subscriber.Name.GivenName+" "+n.Resource.Subscriber.Name.SurName)

	if n.EventType != "BILLING.SUBSCRIPTION.CREATED" {
		return
	}

	newMember, err := api.MemberServer.MemberService.GetMemberFromSubscription(n.Resource.ID)
	if err != nil {
		log.Errorf("error parsing member from webhook: %v", err)
	}

	// Paypal will send us subscriptionID before they actually process the subscription payment.
	// Because of this, we may not get payment information right away.
	// Setting them as active allows us to assign an rfid fob that will actually push to the rfid
	// devices.  Their actual membership status will be evaluate the next time the scheduled job runs.
	newMember.Level = uint8(models.Standard)

	log.Printf("member: %v", newMember)

	api.db.ProcessMember(newMember)
}
