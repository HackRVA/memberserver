package database

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
)

// Communication defines an email communication
type Communication struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Subject           string `json:"subject"`
	FrequencyThrottle int    `json:"frequencyThrottle"`
	Template          string `json:"template"`
}

// GetCommunnications returns all communications from the database
func (db *Database) GetCommunications() []Communication {
	rows, err := db.getConn().Query(context.Background(), getCommunications)
	if err != nil {
		log.Errorf("conn.Query failed: %v", err)
	}

	defer rows.Close()

	var communications []Communication

	for rows.Next() {
		var c Communication
		err = rows.Scan(&c.ID, &c.Name, &c.Subject, &c.FrequencyThrottle, &c.Template)
		communications = append(communications, c)
	}
	return communications
}

// GetCommunnication returns all the requested communication from the database
func (db *Database) GetCommunication(name string) (Communication, error) {
	var c Communication
	err := db.getConn().QueryRow(context.Background(), getCommunication, name).
		Scan(&c.ID, &c.Name, &c.Subject, &c.FrequencyThrottle, &c.Template)
	if err != nil {
		return c, err
	}
	return c, nil
}

func (db *Database) GetMostRecentCommunicationToMember(memberId string, commId int) (time.Time, error) {
	var d time.Time
	err := db.getConn().QueryRow(context.Background(), getLastCommunication, memberId, commId).Scan(&d)
	if err != nil {
		return d, err
	}
	return d, nil
}

func (db *Database) LogCommunication(communicationId int, memberId string) error {
	_, err := db.getConn().Exec(context.Background(), logCommunication, communicationId, memberId)
	if err != nil {
		return err
	}
	return nil
}
