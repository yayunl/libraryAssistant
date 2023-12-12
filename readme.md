# LibraryAssistant

## Description
This is a library assistant that helps you to get the checked out books in the library account (Round Rock Public Library) and calculate the total savings you have made by borrowing books from the library instead of buying them.
The application also offers an API to get the list price of a book per its ISBN.

## Installation
1. Clone the repository
2. Install the requirements
3. Run the application
4. Open the browser and go to http://localhost:8080

## Usage
1. Check list price of a book by its ISBN: http://localhost:8080/ISBN/9781603090575
2. (*)Check the list of books checked out by a user: http://localhost:8080/history/1
3. (*)Check the total savings of a user: http://localhost:8080/savings/1   
   *: Note that the last digit in the URLs of Usage #2 and #3 is the pagination number.   
