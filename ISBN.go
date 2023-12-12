package main

import (
	"github.com/PuerkitoBio/goquery"
	"net/http"
	"strings"
)

func IsValidISBN10(isbn string) bool {
	if len(isbn) != 10 {
		return false
	}
	sum := 0
	for i := 0; i < 10; i++ {
		if isbn[i] == 'X' {
			sum += 10 * (10 - i)
		} else {
			sum += int(isbn[i]-'0') * (10 - i)
		}
	}
	return sum%11 == 0
}

func IsValidISBN13(isbn string) bool {
	if len(isbn) != 13 {
		return false
	}
	sum := 0
	for i := 0; i < 13; i++ {
		if i%2 == 0 {
			sum += int(isbn[i] - '0')
		} else {
			sum += 3 * int(isbn[i]-'0')
		}
	}
	return sum%10 == 0
}

func ISBNContent(isbn string) map[string]string {
	resp, err := http.Get("https://isbndb.com/book/" + isbn)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	// Parse the page
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil
	}
	// Find the price
	content := make(map[string]string)
	doc.Find(".book-table .table").Each(func(i int, bookTable *goquery.Selection) {
		bookTable.Find("tr").Each(func(i int, tr *goquery.Selection) {
			val := ""
			des := tr.Find("th:nth-child(1)").Text()
			desp := strings.ToLower(strings.TrimSpace(des))
			switch desp {
			case "isbn":
				val = tr.Find("th:nth-child(2)").Text()
			case "related isbns":
				relatedISBNs := make([]string, 0)
				tr.Find("a").Each(func(i int, s *goquery.Selection) {
					relatedISBNs = append(relatedISBNs, s.Text())
				})
				val = strings.Join(relatedISBNs, ",")
			default:
				val = tr.Find("td").Text()
			}
			content[desp] = strings.TrimSpace(val)
		})
	})
	return content
}
