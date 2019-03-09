package main

import (
	"database/sql"
	"log"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

type (
	Database struct {
		Driver   string `yaml:"driver"`
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		DbName   string `yaml:"dbname"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
	}
)

var db *sql.DB

func dbConnect(c Database) {
	var err error

	dsn := ""
	switch c.Driver {
	case "postgres":
		dsn = "postgres://" + c.User + ":" + c.Password + "@" + c.Host + ":" + c.Port + "/" + c.DbName
	case "mysql":
		dsn = c.User + ":" + c.Password + "@tcp(" + c.Host + ":" + c.Port + ")/" + c.DbName
	}

	db, err = sql.Open(c.Driver, dsn)
	if err != nil {
		log.Fatalf("Database open error", err)
	}
}

const (
	Q_SELECT_CALLS = `SELECT
		UNIX_TIMESTAMP(calldate) AS created_at
		, CASE WHEN direction = 'inbound' THEN src ELSE dst END AS phone_number
		, direction
		, billsec
		, recordingfile
		, uniqueid
		, CASE WHEN direction = 'inbound' THEN substring(substring(dstchannel,1,locate('-',dstchannel,1)-1),locate('\/',dstchannel,1)+1) ELSE src END AS responsible
	FROM cdr 
	WHERE calldate > DATE_SUB(NOW(), INTERVAL 1 DAY)
		AND direction <> ''
		AND direction <> 'internal'
		AND amo_send_status = 0
	ORDER BY uniqueid, calldate
	`
)

func getCallsFromDB() []amoCrmCallsAdd {
	rows, err := db.Query(Q_SELECT_CALLS)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var calls []amoCrmCallsAdd
	for rows.Next() {
		var (
			created_at    int64
			phone_number  string
			direction     string
			billsec       int
			recordingfile string
			uniqueid      string
			record_link   string
			responsible   string
			amo_user      string
		)
		if err := rows.Scan(&created_at, &phone_number, &direction, &billsec, &recordingfile, &uniqueid, &responsible); err != nil {
			log.Fatal(err)
		}
		if recordingfile != "" {
			record_link = config.AmoCRM.RecordUrl + "/" + recordingfile
		}
		if responsible != "" {
			if user, ok := config.AmoCRM.NumberNoUser[responsible]; ok {
				amo_user = user
			}
		}
		calls = append(calls, amoCrmCallsAdd{
			PhoneNumber: phone_number,
			Direction:   direction,
			Duration:    billsec,
			CreatedAt:   created_at,
			Link:        record_link,
			Uniq:        uniqueid,
			Responsible: amo_user,
		})
	}

	return calls
}

const (
	Q_UPDATE_CALL = `UPDATE cdr
 		SET amo_send_status = 1
 		WHERE uniqueid = ?
 	`
)

func setSendCallToDB(calls []amoCrmCallsAdd) {
	stmt, err := db.Prepare(Q_UPDATE_CALL)
	if err != nil {
		log.Fatalln("Q_UPDATE_CALL Error", err)
	}
	defer stmt.Close()

	for _, call := range calls {
		_, err = stmt.Exec(
			call.Uniq,
		)
		if err != nil {
			log.Fatalf("Q_UPDATE_CALL Error: %+v\n", err)
		}
	}
}
