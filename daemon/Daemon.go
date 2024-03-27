package daemon

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
)

type Daemon struct {
	Config        *Config
	Logger        *zerolog.Logger
	DBConn        *sql.DB
	TrackerMaster *TrackerMaster
	Running       bool
}

func NewDaemon(config *Config) *Daemon {
	logger := zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	}).With().Timestamp().Logger()

	db, err := sql.Open(config.DatabaseType, config.DatabasePath)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to open database")
	}

	trackerMaster := NewTrackerMaster(db)
	trackerMaster.AddDefaultTrackers()

	daemon := &Daemon{
		Config:        config,
		Logger:        &logger,
		DBConn:        db,
		TrackerMaster: trackerMaster,
		Running:       true,
	}

	return daemon
}

func (daemon *Daemon) Close() {
	daemon.Logger.Info().Msg("Daemon shutting down")
	daemon.DBConn.Close()
}

func (daemon *Daemon) Start() {
	daemon.Logger.Info().Msg("Daemon starting")

	if daemon.Config.InitialLaunch {
		daemon.Logger.Info().Msg("Initial launch enabled. Configuring trackers")
		err := daemon.TrackerMaster.Initialize()
		if err != nil {
			daemon.Logger.Fatal().Err(err).Msg("Failed to initialize trackers")
		}
		return
	}

	daemon.setupTables()

	http.HandleFunc("/", daemon.rootPath)
	http.HandleFunc("/archive", daemon.archiveRequest)
	http.HandleFunc("/tracker/person", daemon.trackerPersonRequest)

	json, _ := json.Marshal(daemon.Config)
	daemon.Logger.Debug().Msg("Configuration: " + string(json))

	go func() {
		for daemon.Running {
			people, err := daemon.TrackerMaster.QueryTrackedPeople()
			if err != nil {
				daemon.Logger.Error().Err(err).Msg("Failed to query tracked people")
				continue
			}

			for _, person := range people {
				status, err := daemon.TrackerMaster.GetStatus(person.User, person.Place)
				if err != nil {
					daemon.Logger.Error().Err(err).Msg("Failed to get status")
					continue
				}
				daemon.Logger.Info().Str("user", person.User).Str("place", person.Place).Time("status", status).Msg("Got status")

				entry := ArchiveEntry{
					Place:     person.Place,
					User:      person.User,
					Timestamp: status,
				}
				_, err = daemon.InsertIntoArchiveWithTimestamp(entry)
				if err != nil {
					daemon.Logger.Error().Err(err).Msg("Failed to insert into archive")
				}
			}

			time.Sleep(time.Duration(daemon.Config.UpdateInterval) * time.Second)
		}
	}()

	daemon.Logger.Info().Msg("Listening on " + daemon.Config.ListenPath)
	http.ListenAndServe(daemon.Config.ListenPath, nil)
}

func (daemon *Daemon) setupTables() {
	daemon.Logger.Info().Msg("Setting up SQL tables")

	sqlStmt := `
	CREATE TABLE IF NOT EXISTS archive (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		place TEXT,
		user TEXT
	);
	`
	_, err := daemon.DBConn.Exec(sqlStmt)
	if err != nil {
		daemon.Logger.Fatal().Err(err).Msg("Failed to create archive table")
	}

	sqlStmt = `
	CREATE TABLE IF NOT EXISTS people (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user TEXT,
		place TEXT
	);
	`
	_, err = daemon.DBConn.Exec(sqlStmt)
	if err != nil {
		daemon.Logger.Fatal().Err(err).Msg("Failed to create people table")
	}
}

func (daemon *Daemon) InsertIntoArchive(entry ArchiveEntry) (*ArchiveEntry, error) {
	sqlStmt := `INSERT INTO archive (place, user) VALUES (?, ?)`
	_, err := daemon.DBConn.Exec(sqlStmt, entry.Place, entry.User)
	if err != nil {
		daemon.Logger.Error().Err(err).Msg("Failed to insert into archive")
		return nil, err
	}

	// Get the ID of the inserted row
	row := daemon.DBConn.QueryRow("SELECT last_insert_rowid()")
	err = row.Scan(&entry.ID)
	if err != nil {
		daemon.Logger.Error().Err(err).Msg("Failed to get last insert ID")
		return nil, err
	}

	// Get entry using ID
	row = daemon.DBConn.QueryRow("SELECT * FROM archive WHERE id = ?", entry.ID)
	err = row.Scan(&entry.ID, &entry.Timestamp, &entry.Place, &entry.User)
	if err != nil {
		daemon.Logger.Error().Err(err).Msg("Failed to get entry by ID")
		return nil, err
	}

	return &entry, err
}

func (daemon *Daemon) InsertIntoArchiveWithTimestamp(entry ArchiveEntry) (*ArchiveEntry, error) {
	sqlStmt := `INSERT INTO archive (timestamp, place, user) VALUES (?, ?, ?)`
	_, err := daemon.DBConn.Exec(sqlStmt, entry.Timestamp, entry.Place, entry.User)
	if err != nil {
		daemon.Logger.Error().Err(err).Msg("Failed to insert into archive")
		return nil, err
	}

	// Get the ID of the inserted row
	row := daemon.DBConn.QueryRow("SELECT last_insert_rowid()")
	err = row.Scan(&entry.ID)
	if err != nil {
		daemon.Logger.Error().Err(err).Msg("Failed to get last insert ID")
		return nil, err
	}

	// Get entry using ID
	row = daemon.DBConn.QueryRow("SELECT * FROM archive WHERE id = ?", entry.ID)
	err = row.Scan(&entry.ID, &entry.Timestamp, &entry.Place, &entry.User)
	if err != nil {
		daemon.Logger.Error().Err(err).Msg("Failed to get entry by ID")
		return nil, err
	}

	return &entry, err
}

func (daemon *Daemon) LookUpArchiveByUser(user string) ([]ArchiveEntry, error) {
	rows, err := daemon.DBConn.Query("SELECT * FROM archive WHERE user = ?", user)
	if err != nil {
		daemon.Logger.Error().Err(err).Msg("Failed to look up archive")
		return nil, err
	}
	defer rows.Close()

	entries := []ArchiveEntry{}
	for rows.Next() {
		var entry ArchiveEntry
		err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Place, &entry.User)
		if err != nil {
			daemon.Logger.Error().Err(err).Msg("Failed to scan archive")
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (daemon *Daemon) LookUpArchiveByPlace(place string) ([]ArchiveEntry, error) {
	rows, err := daemon.DBConn.Query("SELECT * FROM archive WHERE place = ?", place)
	if err != nil {
		daemon.Logger.Error().Err(err).Msg("Failed to look up archive")
		return nil, err
	}
	defer rows.Close()

	entries := []ArchiveEntry{}
	for rows.Next() {
		var entry ArchiveEntry
		err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Place, &entry.User)
		if err != nil {
			daemon.Logger.Error().Err(err).Msg("Failed to scan archive")
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (daemon *Daemon) LookUpArchiveByPlaceAndUser(place, user string) ([]ArchiveEntry, error) {
	rows, err := daemon.DBConn.Query("SELECT * FROM archive WHERE place = ? AND user = ?", place, user)
	if err != nil {
		daemon.Logger.Error().Err(err).Msg("Failed to look up archive")
		return nil, err
	}
	defer rows.Close()

	entries := []ArchiveEntry{}
	for rows.Next() {
		var entry ArchiveEntry
		err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Place, &entry.User)
		if err != nil {
			daemon.Logger.Error().Err(err).Msg("Failed to scan archive")
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (daemon *Daemon) rootPath(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (daemon *Daemon) archiveRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		daemon.lookupRequestHandler(w, r)
	} else if r.Method == http.MethodPost {
		daemon.addArchiveEntryRequestHandler(w, r)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (daemon *Daemon) trackerPersonRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		daemon.trackerPersonGetHandler(w, r)
	} else if r.Method == http.MethodPost {
		daemon.trackerPersonPostHandler(w, r)
	} else if r.Method == http.MethodDelete {
		daemon.trackerPersonDeleteHandler(w, r)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (daemon *Daemon) trackerPersonGetHandler(w http.ResponseWriter, _ *http.Request) {
	people, err := daemon.TrackerMaster.QueryTrackedPeople()
	if err != nil {
		createErrorForRequest(w, "Failed to query tracked people", daemon.Logger, err)
		return
	}

	jsonData, err := json.Marshal(people)
	if err != nil {
		createErrorForRequest(w, "Failed to marshal tracked people", daemon.Logger, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

func (daemon *Daemon) trackerPersonPostHandler(w http.ResponseWriter, r *http.Request) {
	var person Person

	err := json.NewDecoder(r.Body).Decode(&person)
	if err != nil {
		createErrorForRequest(w, "Failed to decode JSON", daemon.Logger, err)
		return
	}

	if person.User == "" || person.Place == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = daemon.TrackerMaster.AddTrackedPerson(person)
	if err != nil {
		createErrorForRequest(w, "Failed to add tracked person", daemon.Logger, err)
		return
	}

	person_lookup, err := daemon.TrackerMaster.QueryTrackedPerson(person.User)
	if err != nil {
		createErrorForRequest(w, "Failed to query tracked person", daemon.Logger, err)
		return
	}

	jsonData, err := json.Marshal(person_lookup)
	if err != nil {
		createErrorForRequest(w, "Failed to marshal tracked person", daemon.Logger, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

func (daemon *Daemon) trackerPersonDeleteHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("user")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := daemon.TrackerMaster.RemoveTrackedPerson(name)
	if err != nil {
		createErrorForRequest(w, "Failed to remove tracked person", daemon.Logger, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (daemon *Daemon) addArchiveEntryRequestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var entry ArchiveEntry
	err := json.NewDecoder(r.Body).Decode(&entry)
	if err != nil {
		createErrorForRequest(w, "Failed to decode JSON", daemon.Logger, err)
		return
	}

	addedEntry := &ArchiveEntry{}
	if entry.Timestamp.IsZero() {
		addedEntry, err = daemon.InsertIntoArchive(entry)
		if err != nil {
			createErrorForRequest(w, "Failed to insert into archive", daemon.Logger, err)
			return
		}
	} else {
		addedEntry, err = daemon.InsertIntoArchiveWithTimestamp(entry)
		if err != nil {
			createErrorForRequest(w, "Failed to insert into archive", daemon.Logger, err)
			return
		}
	}

	json, err := json.Marshal(addedEntry)
	if err != nil {
		createErrorForRequest(w, "Failed to marshal archive entry", daemon.Logger, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(json)
}

func (daemon *Daemon) lookupRequestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	places := r.URL.Query().Get("place")
	users := r.URL.Query().Get("user")
	if places == "" && users == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	entries := []ArchiveEntry{}
	var err error

	if places != "" && users != "" {
		for _, place := range strings.Split(places, ",") {
			for _, user := range strings.Split(users, ",") {
				entries_both, err := daemon.LookUpArchiveByPlaceAndUser(place, user)
				if err != nil {
					createErrorForRequest(w, "Failed to look up archive entry", daemon.Logger, err)
					return
				}
				entries = append(entries, entries_both...)
			}
		}
	} else {
		if places != "" {
			for _, place := range strings.Split(places, ",") {
				entries_place, err := daemon.LookUpArchiveByPlace(place)
				if err != nil {
					createErrorForRequest(w, "Failed to look up archive entry", daemon.Logger, err)
					return
				}
				entries = append(entries, entries_place...)
			}
		}

		if users != "" {
			for _, user := range strings.Split(users, ",") {
				entries_user, err := daemon.LookUpArchiveByUser(user)
				if err != nil {
					createErrorForRequest(w, "Failed to look up archive entry", daemon.Logger, err)
					return
				}
				entries = append(entries, entries_user...)
			}
		}
	}

	jsonData, err := json.Marshal(entries)
	if err != nil {
		createErrorForRequest(w, "Failed to marshal archive entry", daemon.Logger, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

func createErrorForRequest(w http.ResponseWriter, message string, logger *zerolog.Logger, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	type Error struct {
		Message string `json:"message"`
	}
	error := Error{Message: message}
	if err != nil {
		error.Message = error.Message + ": " + err.Error()
	}
	json, err := json.Marshal(error)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal error")
		return
	}
	w.Write(json)
	logger.Error().Msg(message)
}
