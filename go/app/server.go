package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
	"database/sql"
)

type Server struct {
	// Port is the port number to listen on.
	Port string
	// ImageDirPath is the path to the directory storing images.
	ImageDirPath string
}

// Run is a method to start the server.
// This method returns 0 if the server started successfully, and 1 otherwise.
func (s Server) Run() int {
	// set up logger
	// STEP 4-6: set the log level to DEBUG
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug, //display log messages at the debug level and above.
	}))
	slog.SetDefault(logger)
	
	// set up CORS settings
	frontURL, found := os.LookupEnv("FRONT_URL")
	if !found {
		frontURL = "http://localhost:3000"
	}

	// STEP 5-1: set up the database connection

	// set up handlers
	itemRepo := NewItemRepository()
	h := &Handlers{imgDirPath: s.ImageDirPath, itemRepo: itemRepo, db: db}

	// set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.Hello)
	mux.HandleFunc("POST /items", h.AddItem)
	mux.HandleFunc("GET /items", h.GetItem) // STEP 4-3 implement the GET /items endpoint
	mux.HandleFunc("GET /images/{filename}", h.GetImage)
	mux.HandleFunc("GET /items/{item_id}", h.GetItemByID) //STEP 4-5: implement the GET /items/{item_id} endpoint
	mux.HandleFunc("GET /search", h.SearchItem) //STEP 5-2: implement the GET /search/{keyword} endpoint

	// start the server
	slog.Info("http server started on", "port", s.Port)
	err := http.ListenAndServe(":"+s.Port, simpleCORSMiddleware(simpleLoggerMiddleware(mux), frontURL, []string{"GET", "HEAD", "POST", "OPTIONS"}))
	if err != nil {
		slog.Error("failed to start server: ", "error", err)
		return 1
	}

	return 0
}

type Handlers struct {
	// imgDirPath is the path to the directory storing images.
	imgDirPath string
	itemRepo   ItemRepository
	db         *sql.DB
}

type HelloResponse struct {
	Message string `json:"message"`
}

// Hello is a handler to return a Hello, world! message for GET / .
func (s *Handlers) Hello(w http.ResponseWriter, r *http.Request) {
	resp := HelloResponse{Message: "Hello, world!"}
	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type AddItemRequest struct {
	Name string `form:"name"`
	Category string `form:"category"` // STEP 4-2: add a category field
	Image []byte `form:"image"` // STEP 4-4: add an image field
}

type AddItemResponse struct {
	Message string `json:"message"`
}

// parseAddItemRequest parses and validates the request to add an item.
func parseAddItemRequest(r *http.Request) (*AddItemRequest, error) {
	req := &AddItemRequest{
		Name: r.FormValue("name"),
		Category: r.FormValue("category"), // STEP 4-2: add a category field
	}

	// STEP 4-4: add an image field
	file, _, err := r.FormFile("image")
 	if err != nil {
 		return nil, fmt.Errorf("failed to get image: %w", err)
 	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}
	req.Image = imageData

	// validate the request
	if req.Name == "" {
		return nil, errors.New("name is required")
	}

	// STEP 4-2: validate the category field
	if req.Category == "" {
		return nil, errors.New("category is requred")
	}

	// STEP 4-4: validate the image field
	if len(req.Image) == 0 {
		return nil, errors.New("image is requred")
	}

	return req, nil
}

// AddItem is a handler to add a new item for POST /items .
func (s *Handlers) AddItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, err := parseAddItemRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// STEP 4-4: uncomment on adding an implementation to store an image
	fileName, err := s.storeImage(req.Image)
	if err != nil {
		slog.Error("failed to store image: ", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	imageFileName := filepath.Base(fileName)

	item := &Item{
		Name: req.Name,
		Category: req.Category, // STEP 4-2: add a category field
		Image: imageFileName, // STEP 4-4: add an image field
	}
	message := fmt.Sprintf("item received: %s", item.Name)
	slog.Info(message)

	// STEP 4-2: add an implementation to store an item
	err = s.itemRepo.Insert(ctx, item)
	if err != nil {
		slog.Error("failed to store item: ", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := AddItemResponse{Message: message}
	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type GetItemResponse struct {
	Items []Item `json:"items"`
}

// GetItem is a handler to show items stored in images.json for GET /items .
func (s *Handlers) GetItem(w http.ResponseWriter, r *http.Request) {
	items, err := s.itemRepo.LoadFromDatabase() //use ItemRepository
	if err != nil {
		slog.Error("Failed to load items: ", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	resp := GetItemResponse{Items: items} //this is the data returned as the response
	err = json.NewEncoder(w).Encode(resp) //encode resp into JSON format and writes it to the HTTP response (w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

//only returns one item
type GetItemByIDResponse struct {
	Item Item `json:"item"`
}

func (s *Handlers) GetItemByID(w http.ResponseWriter, r *http.Request) {
	//get the item_id from the path parameter
	idStr := r.PathValue("item_id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	items, err := s.itemRepo.LoadFromDatabase()
	if err != nil {
		slog.Error("failed to load items: ", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}	

	var foundItem *Item
	for _, item := range items {
		if item.ID == id {
			foundItem = &item
			break
		}
	}

	if foundItem == nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	resp := GetItemByIDResponse{Item: *foundItem}
	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

var SearchItemResponse struct {
	Item Item `json:"item"`
}

func (s *Handlers) SearchItem(w http.ResponseWriter, r *http.Request) {
	//get the keyword from the query parameter
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		http.Error(w, "Keyword is required", http.StatusBadRequest)
		return
	}

	//use "LIKE" to search for items that contain the keyword
	rows, err := s.db.Query(`
		SELECT items.id, items.name, categories.name AS category, items.image_name
		FROM items
		JOIN categories ON items.category_id = categories.id
		WHERE items.name LIKE ?`, "%"+keyword+"%")
	
	if err != nil {
		slog.Error("items not found: ", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var item Item
		err := rows.Scan(&item.ID, &item.Name, &item.Category, &item.Image)
		if err != nil {
			slog.Error("failed to scan item: ", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// storeImage stores an image and returns the file path and an error if any.
// this method calculates the hash sum of the image as a file name to avoid the duplication of a same file
// and stores it in the image directory.
func (s *Handlers) storeImage(image []byte) (filePath string, err error) {
	// STEP 4-4: add an implementation to store an image
	// TODO:
	// - calc hash sum
	hash := sha256.Sum256(image)
	hashedValue := hex.EncodeToString(hash[:])
	fileName := hashedValue + ".jpg"

	// - build image file path
	filePath = filepath.Join(s.imgDirPath, fileName)

	// - check if the image already exists
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	// - store image
	if err := StoreImage(s.imgDirPath,fileName, image); err != nil {
		return "", err
	}
	
	// - return the image file path
	return filePath, nil
}

type GetImageRequest struct {
	FileName string // path value
}

// parseGetImageRequest parses and validates the request to get an image.
func parseGetImageRequest(r *http.Request) (*GetImageRequest, error) {
	req := &GetImageRequest{
		FileName: r.PathValue("filename"), // from path parameter
	}

	// validate the request
	if req.FileName == "" {
		return nil, errors.New("filename is required")
	}

	return req, nil
}

// GetImage is a handler to return an image for GET /images/{filename} .
// If the specified image is not found, it returns the default image.
func (s *Handlers) GetImage(w http.ResponseWriter, r *http.Request) {
	req, err := parseGetImageRequest(r)
	if err != nil {
		slog.Warn("failed to parse get image request: ", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	imgPath, err := s.buildImagePath(req.FileName)
	if err != nil {
		if !errors.Is(err, errImageNotFound) {
			slog.Warn("failed to build image path: ", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// when the image is not found, it returns the default image without an error.
		slog.Debug("image not found", "filename", imgPath)
		imgPath = filepath.Join(s.imgDirPath, "default.jpg")
	}

	slog.Info("returned image", "path", imgPath)
	http.ServeFile(w, r, imgPath)
}

// buildImagePath builds the image path and validates it.
func (s *Handlers) buildImagePath(imageFileName string) (string, error) {
	imgPath := filepath.Join(s.imgDirPath, filepath.Clean(imageFileName))

	// to prevent directory traversal attacks
	rel, err := filepath.Rel(s.imgDirPath, imgPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid image path: %s", imgPath)
	}

	// validate the image suffix
	if !strings.HasSuffix(imgPath, ".jpg") && !strings.HasSuffix(imgPath, ".jpeg") {
		return "", fmt.Errorf("image path does not end with .jpg or .jpeg: %s", imgPath)
	}

	// check if the image exists
	_, err = os.Stat(imgPath)
	if err != nil {
		return imgPath, errImageNotFound
	}

	return imgPath, nil
}
