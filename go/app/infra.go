package app

import (
	"context"
	"errors"
	"os"
	"encoding/json"
	"path/filepath"
	"fmt"
	// STEP 5-1: uncomment this line
	// _ "github.com/mattn/go-sqlite3"
)

var errImageNotFound = errors.New("image not found")

type Item struct {
	ID   int    `db:"id" json:"-"`
	Name string `db:"name" json:"name"`
	Category string `db:"category" json:"category"`
	Image string `db:"image" json:"image"`
}

//to add items under "items" key
type ItemList struct {
	Items []Item `json:"items"`
}

// Please run `go generate ./...` to generate the mock implementation
// ItemRepository is an interface to manage items.
//
//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -package=${GOPACKAGE} -destination=./mock_$GOFILE
type ItemRepository interface {
	Insert(ctx context.Context, item *Item) error
	LoadFromJSONFile() ([]Item, error)
}

// itemRepository is an implementation of ItemRepository
type itemRepository struct {
	// fileName is the path to the JSON file storing items.
	fileName string
}

// NewItemRepository creates a new itemRepository.
func NewItemRepository() ItemRepository {
	return &itemRepository{fileName: "items.json"}
}

// Insert inserts an item into the repository.
func (i *itemRepository) Insert(ctx context.Context, item *Item) error {
	// STEP 4-2: add an implementation to store an item
	// Open the file in read-write mode (if it does't exist create an empty file)
	file, err := os.OpenFile(i.fileName, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	//get current item list, itemList struct is in infla.go
	var itemList ItemList

	//if the file is not empty, decode itemList
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&itemList)
	if err != nil && err.Error() != "EOF" { // if it is empty, ignore EOF error
		return err
	}

	// add new item
	itemList.Items = append(itemList.Items, *item)

	//move to the beginning of the file to overwrite the content
	file.Seek(0, 0)
	file.Truncate(0)

	//store item list to the file again
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // set the indentation
	if err := encoder.Encode(itemList); err != nil {
		return err
	}

	return nil
}

func (i *itemRepository) LoadFromJSONFile() ([]Item, error) {
	file, err := os.Open(i.fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// get current item list
	var itemList ItemList

	//decode itemList
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&itemList)
	if err != nil {
		return nil, err
	}

	return itemList.Items, nil
}

// StoreImage stores an image and returns an error if any.
// This package doesn't have a related interface for simplicity.
func StoreImage(dirPath string, fileName string, image []byte) error {
	// STEP 4-4: add an implementation to store an image
	filePath := filepath.Join(dirPath, fileName)
	
	//write image to the file
	if err := os.WriteFile(filePath, image, 0666); err != nil {
		return fmt.Errorf("failed to write image file: %w", err)
	}

	// Return nil if everything succeeds
	return nil
}
