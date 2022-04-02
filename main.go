package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"thinklink/utils"
	"time"

	// "github.com/joho/godotenv"
	"github.com/mattn/go-sqlite3"
	_ "github.com/mattn/go-sqlite3"
)

var (
	database *sql.DB
	min      int
	max      int
	host     string = "http://localhost:8080"
	email    Email
)

type Email struct {
	from, body string
	to         []string
}
type Bitcoin struct {
	MarketData struct {
		CurrentPrice struct {
			USD float64 `json:"usd"`
		} `json:"current_price"`
	} `json:"market_data"`
}

func init() {
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal("Error loading .env file")
	// }
	email.from = os.Getenv("from")
	email.to = []string{os.Getenv("email")}
	min, _ = strconv.Atoi(os.Getenv("min"))
	max, _ = strconv.Atoi(os.Getenv("max"))
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

		ticker := time.NewTicker(time.Second * 30)
		for {
			select {
			case <-ticker.C:
				go fetchPrice("bitcoin", "", time.Now().Format("02-01-2006"), int(time.Now().UnixMilli()))
			}
		}
	}()

	http.HandleFunc("/api/prices/btc", handleRequest)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		utils.SendErrorResponseToClient(w, http.StatusNotFound, "Requested path is not found")
	})
	log.Fatal(http.ListenAndServe(":8080", nil))

}

type PriceRow struct {
	Timestamp int
	Price     float64
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
		date := r.URL.Query().Get("date")

		// validate query params
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil {
			utils.SendErrorResponseToClient(w, http.StatusBadRequest, "Unable to parse limit")
			return
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		if err != nil {
			utils.SendErrorResponseToClient(w, http.StatusBadRequest, "Unable to parse offset")
			return
		}
		utcDate, err := time.Parse("02-01-2006", date)
		if err != nil {
			utils.SendErrorResponseToClient(w, http.StatusBadRequest, "Unable to parse date")
			return
		}
		var count int
		err = database.QueryRow("SELECT count(*) as count from prices where `date` = ?", date).Scan(&count)

		if err != nil {
			log.Println(err)
		}
		// if date in query param is older fetch records and store in db then send response
		if count == 0 {
			fetchPrice("bitcoin", fmt.Sprintf("/history?date=%s", date), date, int(utcDate.UnixMilli()))
			//count is increased since getting data from history path returns 1 price
			count = 1
		}

		rows, err := database.Query("SELECT timestamp,name,price,currency FROM prices where `date` = ? LIMIT ? OFFSET ?", date, limit, offset)

		if err != nil {
			utils.SendErrorResponseToClient(w, http.StatusInternalServerError, "Try again later")
			return
		}
		defer rows.Close()
		priceResponse := PriceResponse{
			Data:  make([]PriceRow, 0),
			Count: count,
			Url:   host + r.URL.String(),
			Next:  fmt.Sprintf("%s/api/prices/btc?date=%s&offset=%d&limit=%d", host, date, offset+limit, limit),
		}
		for rows.Next() {
			var timestamp int
			var price float64
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
	id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	price REAL NOT NULL,
	date TEXT  NOT NULL,
	timestamp INTEGER NOT NULL,
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

func isToday(date string) bool {
	dp, _ := time.Parse("01-02-2006", date)
	today := time.Now()
	year, month, day := dp.Date()
	return year == today.Year() && month == today.Month() && day == today.Day()
}

// fetchPrice calls the external api based on id and path,
// either today's data is fetched or any historical data.

func fetchPrice(id, path, date string, timestamp int) {
	resp, err := http.Get(fmt.Sprintf("https://api.coingecko.com/api/v3/coins/%s%s", id, path))
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
		log.Println("Failed to parse json", err)
		return
	}

	// if bitcoin values goes above max or go below min and requested date is of today then send email
	if bitcoin.MarketData.CurrentPrice.USD < float64(min) && isToday(date) {
		go sendEmail(bitcoin.MarketData.CurrentPrice.USD, fmt.Sprintf("Price of bitcoin went below %d", min))
	}
	if bitcoin.MarketData.CurrentPrice.USD > float64(max) && isToday(date) {
		go sendEmail(bitcoin.MarketData.CurrentPrice.USD, fmt.Sprintf("Price of bitcoin went above %d", max))
	}
	statement, err := database.Prepare("INSERT INTO prices(price,date,currency,name,timestamp) VALUES(?,?,?,?,?)")
	if err != nil {
		log.Println(err)
		return
	}
	_, err = statement.Exec(bitcoin.MarketData.CurrentPrice.USD, date, "usd", "bitcoin", timestamp)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Println("Added price")
}

func sendEmail(price float64, message string) {

	user := os.Getenv("username")
	password := os.Getenv("password")

	addr := fmt.Sprintf("%s:%s", os.Getenv("host"), os.Getenv("port"))
	host := os.Getenv("host")

	email.body = message

	auth := smtp.PlainAuth("", user, password, host)
	err := smtp.SendMail(addr, auth, email.from, email.to, []byte(createHTMLMessage(email)))

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Email sent successfully")

}

func createHTMLMessage(email Email) string {
	msg := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\r\n"
	msg += fmt.Sprintf("From: %s\r\n", email.from)
	msg += fmt.Sprintf("To: %s\r\n", strings.Join(email.to, ";"))
	msg += fmt.Sprintf("\r\n%s\r\n", email.body)

	return msg
}
