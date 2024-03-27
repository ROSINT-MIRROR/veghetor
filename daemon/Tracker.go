package daemon

import (
	"database/sql"
	"errors"
	"time"

	"veghetor/daemon/Trackers"
)

type Tracker interface {
	Name() string
	GetStatus(user string) (time.Time, error)
	Initialize() error
}

type TrackerMaster struct {
	Trackers []Tracker
	DBConn   *sql.DB
}

type Person struct {
	User  string `json:"user"`
	Place string `json:"place"`
}

func NewTrackerMaster(db *sql.DB) *TrackerMaster {
	return &TrackerMaster{
		Trackers: []Tracker{},
		DBConn:   db,
	}
}

func (tm *TrackerMaster) Initialize() error {
	for _, tracker := range tm.Trackers {
		err := tracker.Initialize()
		if err != nil {
			return err
		}
	}
	return nil
}

func (tm *TrackerMaster) QueryTrackedPeople() ([]Person, error) {
	rows, err := tm.DBConn.Query("SELECT user, place FROM people")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	people := []Person{}
	for rows.Next() {
		var person Person
		err := rows.Scan(&person.User, &person.Place)
		if err != nil {
			return nil, err
		}
		people = append(people, person)
	}
	return people, nil
}

func (tm *TrackerMaster) QueryTrackedPerson(user string) (Person, error) {
	var person Person
	err := tm.DBConn.QueryRow("SELECT user, place FROM people WHERE user = ?", user).Scan(&person.User, &person.Place)
	return person, err
}

func (tm *TrackerMaster) RemoveTrackedPerson(user string) error {
	_, err := tm.DBConn.Exec("DELETE FROM people WHERE user = ?", user)
	return err
}

func (tm *TrackerMaster) AddTrackedPerson(person Person) error {
	if person.User == "" || person.Place == "" {
		return errors.New("User and place must be set")
	}

	tm.RemoveTrackedPerson(person.User)
	_, err := tm.DBConn.Exec("INSERT INTO people (user, place) VALUES (?, ?)", person.User, person.Place)
	return err
}

func (tm *TrackerMaster) AddTracker(tracker Tracker) {
	tm.Trackers = append(tm.Trackers, tracker)
}

func (tm *TrackerMaster) AddDefaultTrackers() {
	whatsapp := daemon.NewWhatsAppTracker()
	tm.AddTracker(whatsapp)
}

func (tm *TrackerMaster) GetStatus(name, place string) (time.Time, error) {
	for _, tracker := range tm.Trackers {
		if tracker.Name() == place {
			return tracker.GetStatus(name)
		}
	}
	return time.Time{}, errors.New("Tracker not found: " + place)
}

func (tm *TrackerMaster) GetStatuses(user string) (map[string]time.Time, error) {
	statuses := make(map[string]time.Time)
	for _, tracker := range tm.Trackers {
		status, err := tracker.GetStatus(user)
		if err != nil {
			return nil, err
		}
		statuses[tracker.Name()] = status
	}
	return statuses, nil
}
