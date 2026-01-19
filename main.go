package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type PostResponse struct {
	TotalItems      int `json:"total_items"`
	TotalCategories int `json:"total_categories"`
	TotalPrice      int `json:"total_price"`
}

var db *sql.DB

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func main() {
	host := getEnv("POSTGRES_HOST", "localhost")
	port := getEnv("POSTGRES_PORT", "5432")
	user := getEnv("POSTGRES_USER", "validator")
	password := getEnv("POSTGRES_PASSWORD", "val1dat0r")
	dbname := getEnv("POSTGRES_DB", "project-sem-1")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/api/v0/prices", handlePostPrices).Methods("POST")
	r.HandleFunc("/api/v0/prices", handleGetPrices).Methods("GET")

	serverPort := getEnv("SERVER_PORT", "8080")
	log.Printf("Server started on port %s", serverPort)
	log.Fatal(http.ListenAndServe(":"+serverPort, r))
}

func handlePostPrices(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(fileBytes), int64(len(fileBytes)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var csvContent []byte
	for _, f := range zipReader.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			rc, err := f.Open()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			csvContent, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			break
		}
	}

	if csvContent == nil {
		http.Error(w, "csv file not found", http.StatusBadRequest)
		return
	}

	csvReader := csv.NewReader(bytes.NewReader(csvContent))
	records, err := csvReader.ReadAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var totalItems int
	var totalPrice float64
	categories := make(map[string]bool)

	for _, record := range records[1:] {
		if len(record) < 5 {
			continue
		}

		id, err := strconv.Atoi(record[0])
		if err != nil {
			continue
		}

		name := record[1]
		category := record[2]

		price, err := strconv.ParseFloat(record[3], 64)
		if err != nil {
			continue
		}

		createDate := record[4]

		_, err = db.Exec(`INSERT INTO prices (id, create_date, name, category, price) VALUES ($1, $2, $3, $4, $5)`,
			id, createDate, name, category, price)
		if err != nil {
			log.Println(err)
		}

		totalItems++
		totalPrice += price
		categories[category] = true
	}

	response := PostResponse{
		TotalItems:      totalItems,
		TotalCategories: len(categories),
		TotalPrice:      int(totalPrice),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleGetPrices(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, create_date, name, category, price FROM prices ORDER BY id`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var csvBuffer bytes.Buffer
	csvWriter := csv.NewWriter(&csvBuffer)
	csvWriter.Write([]string{"id", "name", "category", "price", "create_date"})

	for rows.Next() {
		var id int
		var createDate time.Time
		var name, category string
		var price float64

		if err := rows.Scan(&id, &createDate, &name, &category, &price); err != nil {
			continue
		}

		csvWriter.Write([]string{
			strconv.Itoa(id),
			name,
			category,
			fmt.Sprintf("%.2f", price),
			createDate.Format("2006-01-02"),
		})
	}
	csvWriter.Flush()

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	fileWriter, err := zipWriter.Create("data.csv")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fileWriter.Write(csvBuffer.Bytes())
	zipWriter.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=data.zip")
	w.Write(zipBuffer.Bytes())
}
