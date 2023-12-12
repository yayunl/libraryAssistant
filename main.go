package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

var sessionCookie string

func init() {
	result, cookie := login()
	if result {
		sessionCookie = cookie
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
		totalPaginationEntry := r.URL.Path[len("/history/"):]
		var totalPagination int
		if totalPaginationEntry == "" {
			totalPagination = 1
		} else {
			totalPage, err := strconv.ParseInt(totalPaginationEntry, 16, 0)
			if err != nil {
				// Handle the error if the conversion fails
				fmt.Println("Error converting string to int:", err)
				return
			}
			totalPagination = int(totalPage)
		}

		if sessionCookie == "" {
			w.Write([]byte("Please login first"))
			return
		}
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
		totalPaginationEntry := r.URL.Path[len("/history/"):]
		var totalPagination int
		if totalPaginationEntry == "" {
			totalPagination = 1
		} else {
			totalPage, err := strconv.ParseInt(totalPaginationEntry, 16, 0)
			if err != nil {
				// Handle the error if the conversion fails
				fmt.Println("Error converting string to int:", err)
				return
			}
			totalPagination = int(totalPage)
		}

		total := totalSavings(sessionCookie, totalPagination)
		w.Write([]byte(fmt.Sprintf("At least saved: $%.2f", total)))
	})
	http.ListenAndServe(":8080", mux)
}
