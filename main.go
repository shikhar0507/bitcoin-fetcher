package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"thinklink/utils"
	"time"

	"github.com/mattn/go-sqlite3"
	_ "github.com/mattn/go-sqlite3"
)

var database *sql.DB

type Bitcoin struct {
	MarketData struct {
		CurrentPrice struct {
			USD int `json:"usd"`
		} `json:"current_price"`
	} `json:"market_data"`
}

func main() {
	os.Remove("database.db")

	file, err := os.Create("database.db")
	if err != nil {
		log.Fatal(err.Error())
	}
	file.Close()
	log.Println("database created")

	database, _ = sql.Open("sqlite3", "./database.db")
	defer database.Close()

	//setup primary table
	setupTables()

	go func() {

		//start a new timer ticker which runs every 30 seconds
		// A goroutine is created to fetch BTC Prices and update the PRICES Table

		ticker := time.NewTicker(time.Second * 3)
		for {
			select {
			case <-ticker.C:
				go fetchPrice()

			}
		}
	}()

	http.HandleFunc("/api/prices/btc", handleRequest)
	log.Fatal(http.ListenAndServe(":8080", nil))

}

type PriceRow struct {
	Timestamp int
	Price     int
	Coin      string
}
type PriceResponse struct {
	Url   string     `json:"url"`
	Next  string     `json:"next"`
	Count int        `json:"count"`
	Data  []PriceRow `json:"data"`
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodOptions:
		utils.HandleOptions(w, fmt.Sprintf("%s", http.MethodGet))
	case http.MethodGet:
		date, limit, offset := r.URL.Query().Get("date"), r.URL.Query().Get("limit"), r.URL.Query().Get("offset")
		fmt.Println("start request")
		utcDate, err := time.Parse("02-01-2006", date)
		if err != nil {
			utils.SendErrorResponseToClient(w, http.StatusBadRequest, "Unable to parse date")
			return
		}

		if limit == "" {
			limit = "100"
		}
		if offset == "" {
			offset = "0"
		}

		rows, err := database.Query("SELECT timestamp,name,price,currency FROM prices where timestamp > ? LIMIT ? OFFSET ?", utcDate.UnixMilli(), limit, offset)

		if err != nil {
			utils.SendErrorResponseToClient(w, http.StatusInternalServerError, "Try again later")
			return
		}
		defer rows.Close()
		priceResponse := PriceResponse{
			Data: make([]PriceRow, 0),
		}
		for rows.Next() {
			var timestamp, price int
			var name, currency string
			rows.Scan(&timestamp, &name, &price, &currency)
			row := PriceRow{
				Timestamp: timestamp,
				Price:     price,
				Coin:      currency,
			}
			priceResponse.Data = append(priceResponse.Data, row)
		}
		utils.SendResponseToClient(w, http.StatusOK, priceResponse)

	default:
		utils.SendErrorResponseToClient(w, http.StatusMethodNotAllowed, "Method Not allowed. Allowed Methods are GET")
	}
}

func setupTables() {
	sql := `CREATE TABLE prices(
	price REAL NOT NULL,
	timestamp INTEGER  NOT NULL,
	currency TEXT NOT NULL,
	name TEXT NOT NULL
	)`
	statement, err := database.Prepare(sql)
	if err != nil {
		log.Fatal(err)
	}
	_, execErr := statement.Exec()
	if execErr != nil {
		log.Fatal(err)
	}
	fmt.Println("Table Created")
}

func getError(err error) sqlite3.Error {
	sqliteErr, ok := err.(sqlite3.Error)
	if ok {
		return sqliteErr
	}
	return sqlite3.Error{}
}

func fetchPrice() {
	resp, err := http.Get("https://api.coingecko.com/api/v3/coins/bitcoin")
	if err != nil {
		log.Println(err)
		return
	}
	var bitcoin Bitcoin
	defer resp.Body.Close()
	byteData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return
	}
	err = json.Unmarshal(byteData, &bitcoin)

	if err != nil {
		log.Println("Failed to parse json")
	}
	statement, err := database.Prepare("INSERT INTO prices(price,timestamp,currency,name) VALUES(?,?,?,?)")
	if err != nil {
		log.Fatal(err)
	}

	_, err = statement.Exec(bitcoin.MarketData.CurrentPrice.USD, time.Now().UnixMilli(), "usd", "bitcoin")
	if err != nil {
		// log.Fatal(err)
		fmt.Println(getError(err).Code, sqlite3.ErrConstraint)
	}

}

func getPrices() {

	rows, err := database.Query("select currency,timestamp,price from prices")

	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c *string
		var r *int
		var t *int
		rows.Scan(&c, &t, &r)
		fmt.Println(*c, *t, *r)
	}
}

func createTable(db *sql.DB) {
	createStudentTableSQL := `CREATE TABLE student (
		"idStudent" integer NOT NULL PRIMARY KEY AUTOINCREMENT,		
		"code" TEXT,
		"name" TEXT,
		"program" TEXT		
	  );` // SQL Statement for Create Table

	log.Println("Create student table...")
	statement, err := db.Prepare(createStudentTableSQL) // Prepare SQL Statement
	if err != nil {
		log.Fatal(err.Error())
	}
	statement.Exec() // Execute SQL Statements
	log.Println("student table created")
}

// We are passing db reference connection from main to our method with other parameters
func insertStudent(db *sql.DB, code string, name string, program string) {
	log.Println("Inserting student record ...")
	insertStudentSQL := `INSERT INTO student(code, name, program) VALUES (?, ?, ?)`
	statement, err := db.Prepare(insertStudentSQL) // Prepare statement.
	// This is good to avoid SQL injections
	if err != nil {
		log.Fatalln(err.Error())
	}
	_, err = statement.Exec(code, name, program)
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func displayStudents(db *sql.DB) {
	row, err := db.Query("SELECT * FROM student ORDER BY name")
	if err != nil {
		log.Fatal(err)
	}
	defer row.Close()
	for row.Next() { // Iterate and fetch the records from result cursor
		var id int
		var code string
		var name string
		var program string
		row.Scan(&id, &code, &name, &program)
		log.Println("Student: ", code, " ", name, " ", program)
	}
}
