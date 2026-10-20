package main

import (
	"log"
	"os"

	"goguide/internal/api"
	"golang.org/x/crypto/bcrypt"
)

type seedSKU struct {
	ID, ProductID int
	Label, Specs  string
	Price         int64
	Stock         int
}

func main() {
	db, err := api.OpenDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	products := []struct {
		ID                                  int
		Category, Brand, Title, Description string
	}{
		{1, "laptop", "DemoTech", "轻薄开发笔记本 14", "16GB 内存，支持 USB-C DP 1.4 视频输出"},
		{2, "laptop", "DemoTech", "高性能开发笔记本 16", "32GB 内存，支持 USB-C DP 1.4 视频输出"},
		{3, "monitor", "ViewLab", "27 英寸 2K 显示器", "USB-C DP 1.4 输入，2560x1440 60Hz"},
		{4, "monitor", "ViewLab", "32 英寸 4K 显示器", "HDMI 2.0 输入，3840x2160 60Hz"},
		{5, "monitor", "ViewLab", "参数待核实显示器", "视频协议和刷新率资料缺失"},
		{6, "dock", "LinkHub", "100W 视频扩展坞", "USB-C DP 1.4 上下行，100W 供电"},
		{7, "dock", "LinkHub", "45W 视频扩展坞", "USB-C DP 1.4 上下行，45W 供电"},
		{8, "dock", "LinkHub", "数据扩展坞", "仅传输数据，不支持视频输出"},
		{9, "laptop", "DemoTech", "均衡开发笔记本 15", "32GB 内存，支持 USB-C DP 1.4 视频输出"},
		{10, "monitor", "ViewLab", "24 英寸 FHD 显示器", "USB-C DP 1.4 输入，1920x1080 60Hz"},
	}
	for _, p := range products {
		_, err = db.Exec("INSERT IGNORE INTO products(id,category,brand,title,description,source_kind) VALUES(?,?,?,?,?,'demo')", p.ID, p.Category, p.Brand, p.Title, p.Description)
		if err != nil {
			log.Fatal(err)
		}
	}
	skus := []seedSKU{
		{1, 1, "16GB/512GB", `{"memory_gb":16,"storage_gb":512,"weight_grams":1300,"video_out":["usb-c-dp-1.4"],"max_width":3840,"max_height":2160,"max_refresh_hz":60,"required_power_w":65,"usb_c_charging":true}`, 499900, 5},
		{2, 2, "32GB/1TB", `{"memory_gb":32,"storage_gb":1024,"weight_grams":2200,"video_out":["usb-c-dp-1.4","hdmi-2.0"],"max_width":3840,"max_height":2160,"max_refresh_hz":60,"required_power_w":90,"usb_c_charging":true}`, 679900, 3},
		{3, 3, "27 英寸 2K 60Hz", `{"video_in":["usb-c-dp-1.4"],"width":2560,"height":1440,"refresh_hz":60}`, 129900, 6},
		{4, 4, "32 英寸 4K 60Hz", `{"video_in":["hdmi-2.0"],"width":3840,"height":2160,"refresh_hz":60}`, 219900, 4},
		{5, 5, "参数待确认", `{"width":2560,"height":1440}`, 99900, 3},
		{6, 6, "100W 版", `{"video_in":["usb-c-dp-1.4"],"video_out":["usb-c-dp-1.4"],"max_width":3840,"max_height":2160,"max_refresh_hz":60,"pd_output_w":100}`, 59900, 5},
		{7, 7, "45W 版", `{"video_in":["usb-c-dp-1.4"],"video_out":["usb-c-dp-1.4"],"max_width":3840,"max_height":2160,"max_refresh_hz":60,"pd_output_w":45}`, 29900, 5},
		{8, 8, "仅数据传输", `{"pd_output_w":100}`, 19900, 5},
		{9, 9, "32GB/512GB", `{"memory_gb":32,"storage_gb":512,"weight_grams":1700,"video_out":["usb-c-dp-1.4"],"max_width":3840,"max_height":2160,"max_refresh_hz":60,"required_power_w":65,"usb_c_charging":true}`, 599900, 4},
		{10, 10, "24 英寸 FHD 60Hz", `{"video_in":["usb-c-dp-1.4"],"width":1920,"height":1080,"refresh_hz":60}`, 69900, 6},
	}
	for _, s := range skus {
		_, err = db.Exec("INSERT IGNORE INTO skus(id,product_id,label,specs,price_cents,available) VALUES(?,?,?,?,?,?)", s.ID, s.ProductID, s.Label, s.Specs, s.Price, s.Stock)
		if err != nil {
			log.Fatal(err)
		}
	}
	email, password := os.Getenv("GOGUIDE_ADMIN_EMAIL"), os.Getenv("GOGUIDE_ADMIN_PASSWORD")
	if email != "" && len(password) >= 8 {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatal(err)
		}
		_, err = db.Exec("INSERT IGNORE INTO users(email,password_hash,role) VALUES(?,?,'admin')", email, string(hash))
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Print("demo catalog seeded; prices and inventory are fictional")
}
