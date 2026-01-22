package main

import (
	"archive/tar"
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

type PriceRecord struct {
	Name       string
	Category   string
	Price      float64
	CreateDate string
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
		panic(err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		panic(err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/api/v0/prices", handlePostPrices).Methods("POST")
	r.HandleFunc("/api/v0/prices", handleGetPrices).Methods("GET")

	serverPort := getEnv("SERVER_PORT", "8080")
	log.Printf("Server started on port %s", serverPort)
	if err := http.ListenAndServe(":"+serverPort, r); err != nil {
		panic(err)
	}
}

func extractCSVFromZip(data []byte) ([]byte, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	for _, f := range zipReader.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			return content, nil
		}
	}
	return nil, fmt.Errorf("csv not found")
}

func extractCSVFromTar(data []byte) ([]byte, error) {
	tarReader := tar.NewReader(bytes.NewReader(data))

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if strings.HasSuffix(strings.ToLower(header.Name), ".csv") {
			return io.ReadAll(tarReader)
		}
	}
	return nil, fmt.Errorf("csv not found")
}

func parseAndValidateCSV(csvContent []byte) ([]PriceRecord, error) {
	csvReader := csv.NewReader(bytes.NewReader(csvContent))
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("csv is empty")
	}

	var validRecords []PriceRecord

	for _, record := range records[1:] {
		if len(record) < 5 {
			continue
		}

		name := strings.TrimSpace(record[1])
		category := strings.TrimSpace(record[2])
		priceStr := strings.TrimSpace(record[3])
		dateStr := strings.TrimSpace(record[4])

		if name == "" || category == "" || priceStr == "" || dateStr == "" {
			continue
		}

		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			continue
		}

		validRecords = append(validRecords, PriceRecord{
			Name:       name,
			Category:   category,
			Price:      price,
			CreateDate: dateStr,
		})
	}

	return validRecords, nil
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

	archiveType := r.URL.Query().Get("type")
	if archiveType == "" {
		archiveType = "zip"
	}

	var csvContent []byte
	switch archiveType {
	case "zip":
		csvContent, err = extractCSVFromZip(fileBytes)
	case "tar":
		csvContent, err = extractCSVFromTar(fileBytes)
	default:
		http.Error(w, "unsupported archive type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	validRecords, err := parseAndValidateCSV(csvContent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	insertedCount := 0
	for _, rec := range validRecords {
		_, err = tx.Exec(
			`INSERT INTO prices (name, category, price, create_date) VALUES ($1, $2, $3, $4)`,
			rec.Name, rec.Category, rec.Price, rec.CreateDate)
		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		insertedCount++
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var totalCategories int
	var totalPrice float64
	err = db.QueryRow(`SELECT COUNT(DISTINCT category), COALESCE(SUM(price), 0) FROM prices`).Scan(&totalCategories, &totalPrice)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := PostResponse{
		TotalItems:      insertedCount,
		TotalCategories: totalCategories,
		TotalPrice:      int(totalPrice),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("error encoding response: %v", err)
	}
}

func handleGetPrices(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, name, category, price, create_date FROM prices ORDER BY id`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var csvBuffer bytes.Buffer
	csvWriter := csv.NewWriter(&csvBuffer)

	if err := csvWriter.Write([]string{"id", "name", "category", "price", "create_date"}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for rows.Next() {
		var id int
		var name, category string
		var price float64
		var createDate time.Time

		if err := rows.Scan(&id, &name, &category, &price, &createDate); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := csvWriter.Write([]string{
			strconv.Itoa(id),
			name,
			category,
			fmt.Sprintf("%.2f", price),
			createDate.Format("2006-01-02"),
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	fileWriter, err := zipWriter.Create("data.csv")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := fileWriter.Write(csvBuffer.Bytes()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := zipWriter.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=data.zip")
	if _, err := w.Write(zipBuffer.Bytes()); err != nil {
		log.Printf("error writing response: %v", err)
	}
}
