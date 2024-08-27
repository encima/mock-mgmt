package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type RequestItem struct {
	ID      uint                   `gorm:"primaryKey" json:"id"`
	Path    string                 `json:"path"`
	Data    map[string]interface{} `gorm:"-" json:"data"`
	RawData string                 `gorm:"type:json" json:"-"`
}

func (item *RequestItem) BeforeSave(tx *gorm.DB) (err error) {
	rawData, err := json.Marshal(item.Data)
	if err != nil {
		return err
	}
	item.RawData = string(rawData)
	return nil
}

func (item *RequestItem) AfterFind(tx *gorm.DB) (err error) {
	return json.Unmarshal([]byte(item.RawData), &item.Data)
}

func main() {
	data, err := ioutil.ReadFile("openapi.json")
	if err != nil {
		log.Fatalf("Failed to read OpenAPI file: %v", err)
	}

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		log.Fatalf("Failed to parse OpenAPI file: %v", err)
	}

	router := gin.Default()
	db, err := gorm.Open(sqlite.Open("requests.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(&RequestItem{}); err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err)
	}

	for path, pathItem := range doc.Paths.Map() {
		for method, _ := range pathItem.Operations() {
			switch method {
			case http.MethodPost:
				router.POST(path, func(c *gin.Context) {
					var requestBody map[string]interface{}
					if err := c.ShouldBindJSON(&requestBody); err != nil {
						c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
						return
					}

					item := RequestItem{
						Path: path,
						Data: requestBody,
					}
					if err := db.Create(&item).Error; err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
						return
					}
					c.JSON(http.StatusOK, item)
				})
			case http.MethodGet:
				router.GET(path, func(c *gin.Context) {
					// Retrieve request body from database
					var items []RequestItem
					if err := db.Where("path = ?", path).Find(&items).Error; err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
						return
					}
					c.JSON(http.StatusOK, items)
				})
			}
		}
	}

	// Start the server
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
