package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

var (
	sessionCookie   string
	totalPagination int
)

func init() {
	result, cookie := login()
	if result {
		sessionCookie = cookie
		fmt.Println("Login successfully")
		// Get the total pagination
		var err error
		totalPagination, err = extractTotalPagination(sessionCookie)
		if err != nil {
			panic(errors.New("cannot get total pagination"))
		}
	} else {
		panic("Login failed")
	}

}

func main() {
	// create a multiplexer
	mux := http.NewServeMux()
	// register a handler for the / route
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome to Library Assistant!"))
	})
	// register a handler for the /isbn/ route
	mux.HandleFunc("/isbn/", func(w http.ResponseWriter, r *http.Request) {
		// get the isbn from the URL
		isbn := r.URL.Path[len("/isbn/"):]
		// get the book price and other items
		res := ISBNContent(isbn)

		// convert the map to JSON
		resJson, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(resJson)
	})
	// Get the history of checked out books
	mux.HandleFunc("/history/", func(w http.ResponseWriter, r *http.Request) {
		// get the pagination from the URL
		books := readBookHistoryList(sessionCookie, totalPagination)
		// convert the map to JSON
		resJson, err := json.MarshalIndent(books, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(resJson)
	})

	// Get total savings
	mux.HandleFunc("/savings/", func(w http.ResponseWriter, r *http.Request) {
		// get the pagination from the URL
		startTime := time.Now()

		total := calculateTotalSavings(sessionCookie, totalPagination)
		endTime := time.Now()
		totalTime := endTime.Sub(startTime)
		var response = map[string]interface{}{
			"message": "Total savings in USD",
			"total":   fmt.Sprintf("$%.2f", total),
			"time":    fmt.Sprintf("%vs", totalTime),
		}
		// convert the map to JSON
		resJson, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(resJson)
	})

	// Get the checked out books
	mux.HandleFunc("/due/", func(w http.ResponseWriter, r *http.Request) {
		books, _ := checkedOutBooks(sessionCookie)
		response := map[string]interface{}{
			"message": "Checked-out books",
			"books":   books,
		}

		jsonResponse, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Println("Error marshaling JSON response:", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse)
	})

	http.ListenAndServe(":8080", mux)
}
