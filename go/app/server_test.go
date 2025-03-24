package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"

	"bytes"
	"mime/multipart"
	"errors"
	"encoding/json"
	"database/sql"
	"os"
)

func TestParseAddItemRequest(t *testing.T) {
	t.Parallel()

	type wants struct {
		req *AddItemRequest
		err bool
	}

	// STEP 6-1: define test cases
	cases := map[string]struct {
		args map[string]string
		image []byte
		wants
	}{
		"ok: valid request": {
			args: map[string]string{
				"name":     "used iPhone 16e", // fill here
				"category": "phone", // fill here
			},
			image: []byte("test image"),
			wants: wants{
				req: &AddItemRequest{
					Name: "used iPhone 16e", // fill here
					Category: "phone", // fill here
					Image: []byte("test image"),
				},
				err: false,
			},
		},
		"ng: empty request": {
			args: map[string]string{},
			wants: wants{
				req: nil,
				err: true,
			},
		},
	}

	for name, tt := range cases {
		tt := tt
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)

			// add form fields
			for k, v := range tt.args {
				_ = writer.WriteField(k, v)
			}

			if tt.image != nil {
				part, err := writer.CreateFormFile("image", "test.jpg")
				if err != nil {
					t.Fatalf("failed to create form file: %v", err)
				}
				_, _ = part.Write(tt.image)
			}
			writer.Close()

			// prepare HTTP request
			req, err := http.NewRequest("POST", "http://localhost:9000/items", body)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", writer.FormDataContentType())

			// execute test target
			got, err := parseAddItemRequest(req)

			// confirm the result
			if err != nil {
				if !tt.err {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if diff := cmp.Diff(tt.wants.req, got); diff != "" {
				t.Errorf("unexpected request (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHelloHandler(t *testing.T) {
	t.Parallel()

	// Please comment out for STEP 6-2
	// predefine what we want
	type wants struct {
		code int               // desired HTTP status code
		body map[string]string // desired body
	}
	want := wants{
		code: http.StatusOK,
		body: map[string]string{"message": "Hello, world!"},
	}

	// set up test
	req := httptest.NewRequest("GET", "/hello", nil)
	res := httptest.NewRecorder()

	h := &Handlers{}
	h.Hello(res, req)

	// STEP 6-2: confirm the status code
	if res.Code != want.code {
		t.Errorf("expected status code %d, got %d", want.code, res.Code)
	}

	// STEP 6-2: confirm response body
	var gotBody map[string]string
	if err := json.NewDecoder(res.Body).Decode(&gotBody); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if diff := cmp.Diff(want.body, gotBody); diff != "" {
		t.Errorf("unexpected response body (-want +got):\n%s", diff)
	}
}

func TestAddItem(t *testing.T) {
	t.Parallel()

	type wants struct {
		code int
	}
	cases := map[string]struct {
		args     map[string]string
		injector func(m *MockItemRepository)
		wants
	}{
		"ok: correctly inserted": {
			args: map[string]string{
				"name":     "used iPhone 16e",
				"category": "phone",
			},
			injector: func(m *MockItemRepository) {
				// STEP 6-3: define mock expectation
				// succeeded to insert
				m.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil)
			},
			wants: wants{
				code: http.StatusOK,
			},
		},
		"ng: failed to insert": {
			args: map[string]string{
				"name":     "used iPhone 16e",
				"category": "phone",
			},
			injector: func(m *MockItemRepository) {
				// STEP 6-3: define mock expectation
				// failed to insert
				m.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(errors.New("failed to insert"))
			},
			wants: wants{
				code: http.StatusInternalServerError,
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			mockIR := NewMockItemRepository(ctrl)
			tt.injector(mockIR)
			h := &Handlers{itemRepo: mockIR}

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			
			for k, v := range tt.args {
				_ = writer.WriteField(k, v)
			}

			part, err := writer.CreateFormFile("image", "test.jpg")
			if err != nil {
				t.Fatalf("failed to create form file: %v", err)
			}
			_, _ = part.Write([]byte("test image"))
			writer.Close()

			req := httptest.NewRequest("POST", "/items", body)
			req.Header.Set("Content-Type", writer.FormDataContentType())

			rr := httptest.NewRecorder()
			h.AddItem(rr, req)

			if tt.wants.code != rr.Code {
				t.Errorf("expected status code %d, got %d", tt.wants.code, rr.Code)
			}
			if tt.wants.code >= 400 {
				return
			}

			var respBody AddItemResponse
			if err := json.NewDecoder(rr.Body).Decode(&respBody); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}
			if !strings.Contains(respBody.Message, tt.args["name"]) {
				t.Errorf("response body does not contain %s, got: %s", tt.args["name"], respBody.Message)
			}
		})
	}
}

// STEP 6-4: uncomment this test
func TestAddItemE2e(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test")
	}

	dbTest, fileName, closers, err := setupDB(t)
	if err != nil {
		t.Fatalf("failed to set up database: %v", err)
	}

	db = dbTest

	t.Cleanup(func() {
		for _, c := range closers {
			c()
		}
	})

	type wants struct {
		code int
	}
	cases := map[string]struct {
		args map[string]string
		wants
	}{
		"ok: correctly inserted": {
			args: map[string]string{
				"name":     "used iPhone 16e",
				"category": "phone",
			},
			wants: wants{
				code: http.StatusOK,
			},
		},
		"ng: failed to insert": {
			args: map[string]string{
				"name":     "",
				"category": "phone",
			},
			wants: wants{
				code: http.StatusBadRequest,
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			h := &Handlers{itemRepo: &itemRepository{
				db: dbTest,
				fileName: fileName,
			}}

			var b bytes.Buffer
			w := multipart.NewWriter(&b)

			for k, v := range tt.args {
				_ = w.WriteField(k, v)
			}
			
			fw, err := w.CreateFormFile("image", "test.jpg")
			if err != nil {
				t.Fatalf("failed to create form file: %v", err)
			}
			_, _ = fw.Write([]byte("test image"))
			w.Close()			
			
			req := httptest.NewRequest("POST", "/items", &b)
			req.Header.Set("Content-Type", w.FormDataContentType())

			rr := httptest.NewRecorder()
			h.AddItem(rr, req)

			// check response
			if tt.wants.code != rr.Code {
				t.Errorf("expected status code %d, got %d", tt.wants.code, rr.Code)
			}
			if tt.wants.code >= 400 {
				return
			}

			if !strings.Contains(rr.Body.String(), tt.args["name"]) {
				t.Errorf("response body does not contain %s, got: %s", tt.args["name"], rr.Body.String())
			}
		
			// STEP 6-4: check inserted data
			row := db.QueryRow(`
				SELECT items.name, categories.name 
				FROM items 
				JOIN categories ON items.category_id = categories.id
				WHERE items.name = ?`, tt.args["name"])

			var (
				name string
				category string
			)
			err = row.Scan(&name, &category)
			if err != nil {
				t.Errorf("failed to query inserted item: %v", err)
			}
			if name != tt.args["name"] || category != tt.args["category"] {
				t.Errorf("unexpected item in DB: got (%s, %s), want (%s, %s)", name, category, tt.args["name"], tt.args["category"])
			}
		})
	}
}

func setupDB(t *testing.T) (db *sql.DB, fileName string, closers []func(), e error) {
	t.Helper()

	defer func() {
		if e != nil {
			for _, c := range closers {
				c()
			}
		}
	}()

	// create a temporary file for e2e testing
	f, err := os.CreateTemp(".", "*.sqlite3")
	if err != nil {
		return nil, "", nil, err
	}
	closers = append(closers, func() {
		f.Close()
		os.Remove(f.Name())
	})
	
	// set up tables
	db, err = sql.Open("sqlite3", f.Name())
	if err != nil {
		return nil, "", nil, err
	}
	closers = append(closers, func() {
		db.Close()
	})

	// TODO: replace it with real SQL statements.
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		return nil, "", nil, err
	}

	cmdCategories := `CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(255) UNIQUE
	)`
	_, err = db.Exec(cmdCategories)
	if err != nil {
		return nil, "", nil, err
	}

	cmdItems := `CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(255),
		category_id INTEGER,
		image_name VARCHAR(255),
		FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
	)`
	_, err = db.Exec(cmdItems)
	if err != nil {
		return nil, "", nil, err
	}

	tempFile, err := os.CreateTemp(".", "test_items_*.json")
	if err != nil {
		return nil, "", nil, err
	}
	closers = append(closers, func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	})

	return db, tempFile.Name(), closers, nil
}
