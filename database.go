package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/ros-tel/amocrm"

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
			, CASE WHEN amo_direction = 'inbound' THEN src ELSE dst END AS phone_number
			, amo_direction
			, billsec
			, record
			, uniqueid
			, CASE WHEN amo_direction = 'inbound' THEN substring(substring(dstchannel,1,locate('-',dstchannel,1)-1),locate('\/',dstchannel,1)+1) ELSE src END AS responsible
		FROM cdr
		WHERE calldate > DATE_SUB(NOW(), INTERVAL 2 DAY)
			AND amo_direction <> ''
			AND amo_direction <> 'internal'
			AND amo_send_status = 0
		ORDER BY uniqueid, calldate
	`
)

func getCallsFromDB() []amocrm.Call {
	rows, err := db.Query(Q_SELECT_CALLS)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var (
		calls []amocrm.Call
		call  amocrm.Call
	)
	for rows.Next() {
		call = amocrm.Call{
			Source: "asterisk",
		}
		var (
			recordingfile string
			responsible   string
		)
		err = rows.Scan(
			&call.CreatedAt,
			&call.Phone,
			&call.Direction,
			&call.Duration,
			&recordingfile,
			&call.Uniq,
			&responsible,
		)
		if err != nil {
			log.Fatal(err)
		}

		if call.Direction != "inbound" && call.Direction != "outbound" {
			log.Printf("[ERROR] Bad direction: %s UID: %s", call.Direction, call.Uniq)
			continue
		}

		if recordingfile != "" {
			if finfo, err := os.Stat(config.AmoCRM.RecordPath + "/" + recordingfile); err == nil {
				if finfo.Size() > 128 {
					call.Link = config.AmoCRM.RecordUrl + recordingfile
				}
			}
		}
		if responsible != "" {
			if user, ok := config.AmoCRM.NumberNoUser[responsible]; ok {
				call.ResponsibleUserID = user
			}
		}
		calls = append(calls, call)
	}

	return calls
}

const (
	Q_UPDATE_CALL = `UPDATE cdr
 		SET amo_send_status = 1
 		WHERE uniqueid = ?
 	`
)

func setSendCallToDB(calls []amocrm.Call) {
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
