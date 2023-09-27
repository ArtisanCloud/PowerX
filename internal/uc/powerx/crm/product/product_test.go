package product

import (
	"PowerX/internal/model/crm/product"
	fmt2 "PowerX/pkg/printx"
	"encoding/json"
	"fmt"
	"testing"
)

func TestBuildTree(t *testing.T) {

	// Initialize ProductSpecific slice
	var productSpecifics []*product.ProductSpecific

	// Data provided
	data := `[
	{
		"options": [
			{
				"id": 1,
				"createdAt": "2023-05-18T15:06:27.446799+08:00",
				"updatedAt": "2023-05-18T15:06:27.446799+08:00",
				"DeletedAt": null,
				"productSpecificId": 1,
				"name": "红色",
				"isActivated": true
			},
			{
				"id": 2,
				"createdAt": "2023-05-18T15:06:27.446799+08:00",
				"updatedAt": "2023-05-18T15:06:27.446799+08:00",
				"DeletedAt": null,
				"productSpecificId": 1,
				"name": "褐色",
				"isActivated": true
			},
			{
				"id": 3,
				"createdAt": "2023-05-18T15:06:27.446799+08:00",
				"updatedAt": "2023-05-18T15:06:27.446799+08:00",
				"DeletedAt": null,
				"productSpecificId": 1,
				"name": "白色",
				"isActivated": false
			}
		],
		"id": 1,
		"createdAt": "2023-05-18T15:06:27.444423+08:00",
		"updatedAt": "2023-05-18T15:06:27.444423+08:00",
		"DeletedAt": null,
		"productId": 1,
		"name": "颜色"
	},
	{
		"options": [
			{
				"id": 4,
				"createdAt": "2023-05-18T15:06:27.446799+08:00",
				"updatedAt": "2023-05-18T15:06:27.446799+08:00",
				"DeletedAt": null,
				"productSpecificId": 2,
				"name": "XL",
				"isActivated": true
			},
			{
				"id": 5,
				"createdAt": "2023-05-18T15:06:27.446799+08:00",
				"updatedAt": "2023-05-18T15:06:27.446799+08:00",
				"DeletedAt": null,
				"productSpecificId": 2,
				"name": "M",
				"isActivated": true
			},
			{
				"id": 6,
				"createdAt": "2023-05-18T15:06:27.446799+08:00",
				"updatedAt": "2023-05-18T15:06:27.446799+08:00",
				"DeletedAt": null,
				"productSpecificId": 2,
				"name": "XXL",
				"isActivated": false
			}
		],
		"id": 2,
		"createdAt": "2023-05-18T15:06:27.444423+08:00",
		"updatedAt": "2023-05-18T15:06:27.444423+08:00",
		"DeletedAt": null,
		"productId": 1,
		"name": "尺码"
	}
]`

	// Unmarshal JSON data
	err := json.Unmarshal([]byte(data), &productSpecifics)
	if err != nil {
		fmt.Println("Error:", err)
	}

	var skus []*product.SKU
	GenerateSKURecursively(&product.Product{SPU: "test"}, productSpecifics, 0, &product.SKU{}, &skus)
	fmt2.Dump(skus)

}
