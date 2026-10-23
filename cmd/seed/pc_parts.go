package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"goguide/internal/domain"
)

type demoPart struct {
	id                                int
	category, brand, title, reference string
	specs                             domain.Specs
	basePrice                         int64 // fictional RMB cents
}

// Titles are shortened from the public listing references. All offer prices,
// availability and compatibility fields below are local demonstration data.
var pcParts = []demoPart{
	{101, "ram", "金百达", "银爵 DDR5 6000 32GB 套条", "https://tao.hooos.com/tag_%E9%87%91%E7%99%BE%E8%BE%BEddr5%206000_1.html", domain.Specs{MemoryGB: 32, MemoryType: "DDR5", MemorySpeedMT: 6000}, 69900},
	{102, "ram", "金百达", "银爵 DDR4 3200 32GB 套条", "https://tao.hooos.com/tag_ddr4%203200%2032g.html", domain.Specs{MemoryGB: 32, MemoryType: "DDR4", MemorySpeedMT: 3200}, 45900},
	{103, "gpu", "七彩虹", "GeForce RTX 4060 8GB 显卡", "https://tao.hooos.com/tag_rtx%204060%208gb.html", domain.Specs{GPUVRAMGB: 8, GPULengthMM: 245, MaxPowerW: 115}, 239900},
	{104, "gpu", "NVIDIA", "GeForce RTX 4070 Ti 12GB 显卡", "https://tao.hooos.com/tag_rtx4070.html", domain.Specs{GPUVRAMGB: 12, GPULengthMM: 320, MaxPowerW: 285}, 499900},
	{105, "monitor", "AOC", "Q27G41XMN 27 英寸 2K 180Hz 显示器", "https://tao.hooos.com/goods_QV2Xky3UQtBWGO3mZrFXXwTatQ-DokRR8IP9jKmwj6hO.html", domain.Specs{VideoIn: []string{"hdmi-2.0"}, Width: 2560, Height: 1440, RefreshHz: 180}, 149900},
	{106, "monitor", "三星", "玄龙骑士 G5 27 英寸 2K 180Hz 显示器", "https://tao.hooos.com/tag_%E6%98%BE%E7%A4%BA%E5%99%A827%E5%AF%B82k180hz_2.html", domain.Specs{VideoIn: []string{"hdmi-2.0"}, Width: 2560, Height: 1440, RefreshHz: 180}, 169900},
	{107, "storage", "西部数据", "WD_BLACK SN770 1TB NVMe 固态硬盘", "https://tao.hooos.com/tag_wd%20black%20sn770%201tb.html", domain.Specs{StorageGB: 1024, StorageInterface: "M.2 NVMe"}, 59900},
	{108, "storage", "十铨", "G50 1TB NVMe PCIe 4.0 固态硬盘", "https://tao.hooos.com/tag_nvme%201tb_1.html", domain.Specs{StorageGB: 1024, StorageInterface: "M.2 NVMe"}, 52900},
	{109, "cpu", "英特尔", "酷睿 i5-13400F 处理器", "https://tao.hooos.com/tag_i5%2013400f.html", domain.Specs{Socket: "LGA1700", CPUModel: "i5-13400F", MaxPowerW: 148}, 99900},
	{110, "cpu", "AMD", "锐龙 R5 7500F 处理器", "https://tao.hooos.com/tag_b650m.html", domain.Specs{Socket: "AM5", CPUModel: "R5 7500F", MaxPowerW: 88}, 109900},
	{111, "cooler", "利民", "PA120 MINI 双塔风冷散热器", "https://tao.hooos.com/goods_YKnRnJ8fptXVQQRBvDUWWYS2t6-jdZqqnCXB6mK8r9cX.html", domain.Specs{SupportedSockets: []string{"LGA1700", "AM5"}, CoolerHeightMM: 135}, 16900},
	{112, "cooler", "利民", "PA120 SE 双塔风冷散热器", "https://tao.hooos.com/tag_%E5%88%A9%E6%B0%91pa120%E5%8F%8C%E5%A1%94.html", domain.Specs{SupportedSockets: []string{"LGA1700", "AM5"}, CoolerHeightMM: 155}, 19900},
	{113, "motherboard", "微星", "B760M GAMING PLUS WIFI DDR5 主板", "https://tao.hooos.com/search?w=B760M", domain.Specs{Socket: "LGA1700", MemoryType: "DDR5", FormFactor: "MATX", SupportedCPUModels: []string{"i5-13400F"}, StorageInterfaces: []string{"M.2 NVMe"}}, 84900},
	{114, "motherboard", "微星", "B650M GAMING PLUS WIFI DDR5 主板", "https://tao.hooos.com/tag_b650m.html", domain.Specs{Socket: "AM5", MemoryType: "DDR5", FormFactor: "MATX", SupportedCPUModels: []string{"R5 7500F"}, StorageInterfaces: []string{"M.2 NVMe"}}, 99900},
	{115, "motherboard", "技嘉", "B760M DDR4 主板", "https://tao.hooos.com/tag_b760m%E4%B8%BB%E6%9D%BF.html", domain.Specs{Socket: "LGA1700", MemoryType: "DDR4", FormFactor: "MATX", SupportedCPUModels: []string{"i5-13400F"}, StorageInterfaces: []string{"M.2 NVMe"}}, 64900},
	{116, "case", "爱国者", "W60 MATX 海景房机箱", "https://tao.hooos.com/tag_%E6%9C%BA%E7%AE%B1%E5%8F%B0%E5%BC%8F%E6%9C%BAmatx_4.html", domain.Specs{SupportedFormFactors: []string{"MATX"}, MaxGPULengthMM: 400, MaxCoolerHeightMM: 165}, 22900},
	{117, "case", "先马", "平头哥 M8 MATX 机箱", "https://tao.hooos.com/tag_%E6%9C%BA%E7%AE%B1%E5%8F%B0%E5%BC%8F%E6%9C%BAmatx_4.html", domain.Specs{SupportedFormFactors: []string{"MATX"}, MaxGPULengthMM: 350, MaxCoolerHeightMM: 155}, 17900},
	{118, "keyboard", "狼蛛", "F3387 87 键无线机械键盘", "https://tao.hooos.com/tag_%E6%9C%BA%E6%A2%B0%E9%94%AE%E7%9B%9887%E9%94%AE%E6%97%A0%E7%BA%BF_6.html", domain.Specs{}, 15900},
	{119, "keyboard", "前行者", "MT87 87 键三模机械键盘", "https://tao.hooos.com/tag_%E6%9C%BA%E6%A2%B0%E9%94%AE%E7%9B%9887%E9%94%AE%E6%97%A0%E7%BA%BF_6.html", domain.Specs{}, 19900},
	{120, "headset", "雷蛇", "北海巨妖标准版 X 有线游戏耳机", "https://tao.hooos.com/category_%E6%9C%89%E7%BA%BF%E6%B8%B8%E6%88%8F%E8%80%B3%E6%9C%BA_12.html", domain.Specs{}, 16900},
	{121, "headset", "漫步者", "G35 头戴式有线游戏耳机", "https://tao.hooos.com/tag_%E5%90%AC%E5%A3%B0%E8%BE%A8%E4%BD%8D_1.html", domain.Specs{}, 22900},
	{122, "gpu", "华硕", "Dual GeForce RTX 4060 OC 8GB 显卡", "https://www.asus.com/uk/motherboards-components/graphics-cards/dual/dual-rtx4060-o8g/techspec/", domain.Specs{GPUVRAMGB: 8, GPULengthMM: 227, MaxPowerW: 115}, 259900},
	{123, "gpu", "微星", "GeForce RTX 4060 VENTUS 2X BLACK 8GB 显卡", "https://us.msi.com/Graphics-Card/GeForce-RTX-4060-VENTUS-2X-BLACK-8G-OC/Specification", domain.Specs{GPUVRAMGB: 8, GPULengthMM: 245, MaxPowerW: 115}, 249900},
	{124, "gpu", "技嘉", "GeForce RTX 4060 WINDFORCE OC 8GB 显卡", "https://www.gigabyte.com/us/Graphics-Card/GV-N4060WF2OC-8GD/sp", domain.Specs{GPUVRAMGB: 8, GPULengthMM: 245, MaxPowerW: 115}, 254900},
	{125, "psu", "海盗船", "RM650e 650W ATX 电源", "https://www.corsair.com/us/en/p/psu/cp-9020302-na/rme-series-rm650e-fully-modular-low-noise-atx-power-supply-cp-9020302-na", domain.Specs{PSUPowerW: 650}, 59900},
	{126, "psu", "海韵", "FOCUS GX-750 750W ATX 电源", "https://seasonic.com/focus-gx/", domain.Specs{PSUPowerW: 750}, 79900},
}

func seedPCParts(db *sql.DB) error {
	for _, part := range pcParts {
		if !domain.ValidCategory(part.category) {
			return fmt.Errorf("invalid category %q", part.category)
		}
		_, err := db.Exec(`INSERT IGNORE INTO products(id,category,brand,title,description,source_kind,source_url,source_captured_at)
			VALUES(?,?,?,?,?,'public_reference_demo',?,?)`, part.id, part.category, part.brand, part.title,
			"名称参考公开商品列表；结构化参数、模拟报价和库存仅用于本地导购演示，不代表来源页面现价。", part.reference, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("product %d: %w", part.id, err)
		}
		specs, err := json.Marshal(part.specs)
		if err != nil {
			return err
		}
		for offer := 0; offer < 3; offer++ {
			id := part.id*10 + offer + 1
			price := part.basePrice * int64([]int{94, 100, 109}[offer]) / 100
			label := fmt.Sprintf("模拟报价 %c · 同型号同规格", 'A'+offer)
			_, err = db.Exec(`INSERT INTO skus(id,product_id,label,specs,price_cents,available) VALUES(?,?,?,?,?,?)
				ON DUPLICATE KEY UPDATE specs=VALUES(specs)`, id, part.id, label, specs, price, 8-offer)
			if err != nil {
				return fmt.Errorf("offer %d: %w", id, err)
			}
			for day := 29; day >= 0; day-- {
				date := time.Now().UTC().AddDate(0, 0, -day).Format("2006-01-02")
				trendPrice := price * int64(100+((id+day*7)%11)-5) / 100
				event := ""
				if day == 12 || day == 24 {
					trendPrice = trendPrice * 91 / 100
					event = "模拟促销场景"
				}
				if day == 0 {
					trendPrice = price
				}
				_, err = db.Exec(`INSERT IGNORE INTO demo_price_snapshots(sku_id,observed_on,price_cents,event_label) VALUES(?,?,?,?)`, id, date, trendPrice, event)
				if err != nil {
					return fmt.Errorf("trend %d %s: %w", id, date, err)
				}
			}
		}
	}
	return nil
}
