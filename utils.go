package main

import (
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type Result struct {
	Success                bool   `json:"success"`
	TwoFactor              bool   `json:"twoFactor"`
	Name                   string `json:"name"`
	Phone                  string `json:"phone"`
	Email                  string `json:"email"`
	HomeLocation           string `json:"homeLocation"`
	HomeLocationId         string `json:"homeLocationId"`
	EnableMaterialsRequest bool   `json:"enableMaterialsRequest"`
}

type loginResp struct {
	Result Result `json:"result"`
}

type Book struct {
	Author      string `json:"author"`
	Title       string `json:"title"`
	Format      string `json:"format"`
	LinkUrl     string `json:"linkUrl"`
	PermanentId string `json:"permanentId"`
	ISBN        string `json:"isbn"`
	ListPrice   string `json:"list price"`
}

type history struct {
	Success bool   `json:"success"`
	Titles  []Book `json:"titles"`
}

func login() (bool, string) {
	postUrl := "https://discovery.roundrocktexas.gov/AJAX/JSON?method=loginUser"
	data := url.Values{
		"username": {USERNAME},
		"password": {PASSWORD},
	}
	resp, err := http.PostForm(postUrl, data)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	loginSuccess := false
	cookie := ""
	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}

		bodyString := string(bodyBytes)
		fmt.Printf("%v\n", bodyString)
		respJson := loginResp{}
		json.Unmarshal(bodyBytes, &respJson)
		loginSuccess = respJson.Result.Success

		// Print the response cookie
		cookies := resp.Cookies()
		for _, c := range cookies {
			if strings.Contains(c.Name, "aspen_session") {
				cookie = c.Value
			}
		}
	}

	return loginSuccess, cookie
}

func readBookHistoryList(cookie string, paginationTotal int) []Book {
	urls := make([]string, paginationTotal+1)
	for i := 1; i < len(urls); i++ {
		urls[i] = fmt.Sprintf("https://discovery.roundrocktexas.gov/MyAccount/AJAX?method=getReadingHistory&patronId=%s&sort=checkedOut&page=%d&readingHistoryFilter=", USERID, i)
	}

	var wg sync.WaitGroup
	wg.Add(len(urls))
	bookChan := make(chan history)

	for _, u := range urls {
		go func(uri string, resCan chan history) {
			respJson := history{}

			defer wg.Done()
			// Create a new request
			req, err := http.NewRequest("GET", uri, nil)
			if err != nil {
				fmt.Println("Error creating request:", err)
				return
			}
			// Add a cookie to the request
			c := &http.Cookie{
				Name:  "aspen_session",
				Value: cookie,
			}
			req.AddCookie(c)

			// Perform the request
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				fmt.Println("Error performing request:", err)
				return
			}
			defer resp.Body.Close()

			// Read the response
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Println("Error reading response:", err)
				return
			}

			json.Unmarshal(bodyBytes, &respJson)

			resCan <- respJson
		}(u, bookChan)
	}

	go func() {
		wg.Wait()
		close(bookChan)
	}()

	// Collect the list of books in the checkout pages
	checkedOutBooks := make([]Book, 0)
	for res := range bookChan {
		if res.Success {
			for _, t := range res.Titles {
				checkedOutBooks = append(checkedOutBooks, t)
			}
		}

	}
	// Get the ISBN of the books
	bookId2ISBNnPrice := make(map[string]map[string]string)
	var wg2 sync.WaitGroup
	var isbnPriceChan = make(chan map[string]string)

	for _, book := range checkedOutBooks {
		wg2.Add(1)
		// Spin up goroutines to extract isbn
		go extractISBNAndListPrice(book, cookie, isbnPriceChan, &wg2)
	}
	go func() {
		wg2.Wait()
		close(isbnPriceChan)
	}()

	// Collect the isbn and price from the channel
	totalBooks := len(checkedOutBooks)
	recvdBooks := 0
	for isbnNPrice := range isbnPriceChan {
		recvdBooks += 1
		bookId2ISBNnPrice[isbnNPrice["id"]] = make(map[string]string)
		bookId2ISBNnPrice[isbnNPrice["id"]]["ISBN"] = isbnNPrice["isbn"]
		bookId2ISBNnPrice[isbnNPrice["id"]]["list price"] = isbnNPrice["list price"]
		percent := float64(recvdBooks) / float64(totalBooks) * 100
		fmt.Printf("Progress: (%.2f%%)\n", percent)
	}
	//fmt.Printf("bookId2 ISBN and Price: %v\n", bookId2ISBNnPrice)

	// Add the ISBN to the book
	finalCheckedOutBooks := make([]Book, len(checkedOutBooks))
	for i, book := range checkedOutBooks {
		book.ISBN = bookId2ISBNnPrice[book.PermanentId]["ISBN"]
		book.ListPrice = bookId2ISBNnPrice[book.PermanentId]["list price"]
		if book.Title == "" {
			book.Title = bookId2ISBNnPrice[book.PermanentId]["title"]
		}
		finalCheckedOutBooks[i] = book
	}
	return finalCheckedOutBooks
}

func extractISBNAndListPrice(book Book, cookie string, resChan chan<- map[string]string, wg *sync.WaitGroup) {
	defer wg.Done()
	u := "https://discovery.roundrocktexas.gov" + book.LinkUrl
	// Create a new request
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		resChan <- nil
	}
	// Add a cookie to the request
	c := &http.Cookie{
		Name:  "aspen_session",
		Value: cookie,
	}
	req.AddCookie(c)

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error performing request:", err)
		resChan <- nil
	}
	defer resp.Body.Close()

	// Parse the page
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		fmt.Println("Error parsing the page:", err)
		resChan <- nil
	}

	// Find the div with class "result-label" containing the ISBN
	isbnDiv := doc.Find("div.result-label:contains('ISBN')")

	// Initialize a slice to store individual ISBNs
	var isbnList []string
	// Iterate over child nodes and extract text
	isbnDiv.Next().Contents().Each(func(i int, s *goquery.Selection) {
		// Split the text based on line breaks
		isbns := strings.Split(strings.TrimSpace(s.Text()), "<br/>")
		for _, isbn := range isbns {
			if isbn != "" {
				isbnList = append(isbnList, isbn)
			}
		}
	})

	// Extract the ISBN value from the sibling div
	//isbnText := isbnDiv.Next().Text()
	// Split the ISBNs based on line breaks <br/>
	isbn := strings.Join(isbnList, ",")

	content := map[string]string{}
	content["id"] = book.PermanentId
	content["isbn"] = isbn
	// Get the list price of the book from the ISBN page
	isbnContent := ISBNContent(isbnList[0])
	content["list price"] = isbnContent["list price"]
	content["title"] = isbnContent["full title"]
	resChan <- content
}

func totalSavings(cookie string, totalPagination int) float64 {
	totalSavedAmnt := 0.0
	books := readBookHistoryList(cookie, totalPagination)
	for _, book := range books {
		if book.ListPrice != "" {
			price := book.ListPrice
			// Define a regular expression to match the pattern "USD $X.X"
			re := regexp.MustCompile(`USD \$([\d.]+)`)
			// Find the match in the input string
			match := re.FindStringSubmatch(price)
			// Extract and convert the matched price to a float64
			p, _ := strconv.ParseFloat(match[1], 64)
			totalSavedAmnt += p
		}
	}
	return totalSavedAmnt
}
