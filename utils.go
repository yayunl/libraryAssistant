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
	"time"
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

		//bodyString := string(bodyBytes)
		//fmt.Printf("%v\n", bodyString)
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

func extractTotalPagination(cookie string) (int, error) {

	// Create a new request
	req, err := http.NewRequest("GET", "https://discovery.roundrocktexas.gov/MyAccount/AJAX?method=getReadingHistory&patronId="+USERID+"&sort=checkedOut&page=1&readingHistoryFilter=", nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return 0, err
	}
	// Add a cookie to the request
	c := &http.Cookie{
		Name:  "aspen_session",
		Value: cookie,
	}
	req.AddCookie(c)

	// Perform the request
	client := &http.Client{
		Timeout: time.Duration(RequestTimeout) * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error performing request:", err)
		return 0, err
	}
	defer resp.Body.Close()

	// Parse the page
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		fmt.Println("Error parsing the page:", err)
		return 0, err
	}

	// Find the pagination number
	var pageStr string
	doc.Find("a[onclick]").Last().Each(func(i int, s *goquery.Selection) {
		pageStr = s.Text()
	})
	re := regexp.MustCompile(`\[(\d+)]`)
	match := re.FindStringSubmatch(pageStr)
	if len(match) > 1 {
		number, _ := strconv.Atoi(match[1])
		return number, nil
	} else {
		return 1, nil
	}
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

	// Get the ISBN by checking the detail page of the book
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
		fmt.Printf("Collecting checked out books. Progress: %.2f%%\n", percent)
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

	// Perform the request to get the ISBN on the detail page of the book
	client := &http.Client{
		Timeout: time.Duration(RequestTimeout) * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error performing request:", err)
		resChan <- nil
		return
	}
	defer resp.Body.Close()

	// Parse the page
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		fmt.Println("Error parsing the page:", err)
		resChan <- nil
		return
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
	if len(isbnList) > 0 {
		isbnContent := ISBNContent(isbnList[0])
		content["list price"] = isbnContent["list price"]
		content["title"] = isbnContent["full title"]
	} else {
		fmt.Printf("No ISBN found for %s\n", book.Title)
		content["list price"] = ""
		content["title"] = book.Title
	}

	resChan <- content
}

func calculateTotalSavings(cookie string, totalPagination int) float64 {
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

// For worker pool implementation

func readBookHistoryList2(cookie string, paginationTotal int) []Book {
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
	return checkedOutBooks
}

func extractISBNAndListPrice2(args []interface{}) (interface{}, error) {
	book := args[0].(*Book)
	cookie := args[1].(string)

	result := map[string]string{}
	// Create a new request to get the book details page
	u := "https://discovery.roundrocktexas.gov" + book.LinkUrl
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil, err
	}
	c := &http.Cookie{
		Name:  "aspen_session",
		Value: cookie,
	}
	req.AddCookie(c)

	// Perform the request
	client := &http.Client{
		Timeout: time.Duration(RequestTimeout) * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error performing request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Parse the page to get the ISBN
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		fmt.Println("Error parsing the page:", err)
		return nil, err
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
	isbn := strings.Join(isbnList, ",")
	book.ISBN = isbn
	if len(isbnList) > 0 {
		isbnContent := ISBNContent(isbnList[0])
		book.ListPrice = isbnContent["list price"]
		book.Title = isbnContent["full title"]
	}
	return result, nil
}

func calculateTotalSavings2(books []Book) float64 {
	totalSavedAmnt := 0.0
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

func checkedOutBooks(cookie string) ([]map[string]string, error) {
	u := "https://discovery.roundrocktexas.gov/MyAccount/AJAX?method=getCheckouts&source=all"
	// Create a new request
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil, err
	}
	// Add a cookie to the request
	c := &http.Cookie{
		Name:  "aspen_session",
		Value: cookie,
	}
	req.AddCookie(c)

	// Perform the request to get the ISBN on the detail page of the book
	client := &http.Client{
		Timeout: time.Duration(RequestTimeout) * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error performing request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Parse the page
	fmt.Println("Response status:", resp.Status)
	// Read the response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return nil, err
	}
	//fmt.Println(string(bodyBytes))

	var respJson map[string]interface{}
	json.Unmarshal(bodyBytes, &respJson)
	checkOuts := respJson["checkouts"].(interface{})
	checkBookStr := checkOuts.(string)
	//fmt.Printf("checkBookStr: %s\n", checkBookStr)

	// Create a new bytes.Reader from the checkBookStr
	b := strings.NewReader(checkBookStr)
	doc, err := goquery.NewDocumentFromReader(b)
	if err != nil {
		fmt.Println("Error parsing the page:", err)
		return nil, err
	}

	// Use CSS selectors to find the book names and due dates
	var dueBooks []map[string]string
	var bookTitles []string
	doc.Find(".result.row").Each(func(i int, s *goquery.Selection) {
		// Extract book name
		bookName := s.Find(".result-title").Text()

		// Extract due date
		dueDate := s.Find(".result-label:contains('Due') + .result-value").Text()

		// Print the results

		found := contains(bookTitles, bookName)
		if !found {
			book := map[string]string{}
			book["title"] = bookName
			book["due date"] = dueDate
			dueBooks = append(dueBooks, book)
			bookTitles = append(bookTitles, bookName)
		}

	})

	return dueBooks, nil
}

func contains(arr []string, target string) bool {
	for _, num := range arr {
		if num == target {
			return true
		}
	}
	return false
}
