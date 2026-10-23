package domain

import (
	"math"
	"strings"
)

// Benchmark values are a dated snapshot of PassMark averages, not a live chart.
// LocalRank is within this demo catalog. PublishedRank is a dated PassMark rank.
type Benchmark struct {
	Model         string `json:"model"`
	Mark          int    `json:"mark"`
	LocalRank     int    `json:"local_rank"`
	LocalCount    int    `json:"local_count"`
	PublishedRank int    `json:"published_rank,omitempty"`
	RankScope     string `json:"rank_scope,omitempty"`
	SourceURL     string `json:"source_url"`
	AsOf          string `json:"as_of"`
}

type ComponentScore struct {
	Category  string     `json:"category"`
	Title     string     `json:"title"`
	Score     float64    `json:"score"`
	Benchmark *Benchmark `json:"benchmark,omitempty"`
	BrandTier string     `json:"brand_tier,omitempty"`
	Reason    string     `json:"reason"`
}

type PlanScore struct {
	Score            float64          `json:"score"`
	ValuePerThousand float64          `json:"value_per_thousand_yuan"`
	Coverage         float64          `json:"coverage"`
	ComparableGroup  string           `json:"comparable_group"`
	Method           string           `json:"method"`
	Components       []ComponentScore `json:"components"`
}

var demoBenchmarks = map[string]Benchmark{
	"i5-13400f":   {Model: "Intel Core i5-13400F", Mark: 24870, LocalRank: 2, LocalCount: 2, PublishedRank: 241, RankScope: "PassMark 台式 CPU / 1595", SourceURL: "https://www.cpubenchmark.net/cpu.php?cpu=Intel+Core+i5-13400F&id=5166", AsOf: "2026-10-23"},
	"r5 7500f":    {Model: "AMD Ryzen 5 7500F", Mark: 26506, LocalRank: 1, LocalCount: 2, PublishedRank: 216, RankScope: "PassMark 台式 CPU / 1595", SourceURL: "https://www.cpubenchmark.net/cpu.php?cpu=AMD+Ryzen+5+7500F", AsOf: "2026-10-23"},
	"rtx 4060":    {Model: "GeForce RTX 4060", Mark: 19481, LocalRank: 2, LocalCount: 2, PublishedRank: 100, RankScope: "PassMark GPU 总榜", SourceURL: "https://www.videocardbenchmark.net/gpu.php?gpu=GeForce+RTX+4060&id=4850", AsOf: "2026-10-14"},
	"rtx 4070 ti": {Model: "GeForce RTX 4070 Ti", Mark: 31510, LocalRank: 1, LocalCount: 2, PublishedRank: 17, RankScope: "PassMark GPU 总榜", SourceURL: "https://www.videocardbenchmark.net/gpu.php?gpu=GeForce+RTX+4070+Ti&id=4699", AsOf: "2026-10-22"},
}

func BenchmarkFor(s SKU) *Benchmark {
	title := strings.ToLower(s.Title)
	keys := []string{"rtx 4070 ti", "rtx 4060", "i5-13400f", "r5 7500f"}
	for _, key := range keys {
		if strings.Contains(title, key) {
			b := demoBenchmarks[key]
			return &b
		}
	}
	return nil
}

// BrandTier is a user-configurable demo preference, not an industry certification.
// GPU silicon benchmark scores remain independent of board-partner tiers.
func BrandTier(category, brand string) string {
	if category != "gpu" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(brand)) {
	case "华硕", "asus", "微星", "msi", "技嘉", "gigabyte":
		return "一线（演示偏好）"
	case "七彩虹", "colorful":
		return "二线（演示偏好）"
	default:
		return "未分层"
	}
}

func ScorePlan(items []SKU, amountCents int64) PlanScore {
	weights := map[string]float64{"cpu": 28, "gpu": 42, "ram": 10, "storage": 10, "monitor": 5}
	all, known, weighted := 0.0, 0.0, 0.0
	parts := make([]ComponentScore, 0, len(items))
	group := ""
	for _, s := range items {
		group += s.Category + "/"
		c := ComponentScore{Category: s.Category, Title: s.Title, BrandTier: BrandTier(s.Category, s.Brand)}
		weight := weights[s.Category]
		if weight == 0 {
			continue
		}
		all += weight
		switch s.Category {
		case "cpu", "gpu":
			c.Benchmark = BenchmarkFor(s)
			if c.Benchmark != nil {
				maxMark := 26506.0
				if s.Category == "gpu" {
					maxMark = 31510
				}
				c.Score = 100 * float64(c.Benchmark.Mark) / maxMark
				c.Reason = "PassMark 样本均值，仅在本项目同品类样本中归一化"
				known += weight
			}
		case "ram":
			if s.Specs.MemoryGB > 0 && s.Specs.MemorySpeedMT > 0 {
				c.Score = math.Min(100, float64(s.Specs.MemoryGB)/32*70+float64(s.Specs.MemorySpeedMT)/6000*30)
				c.Reason = "演示规格分：容量 70%、频率 30%；不是实测性能"
				known += weight
			}
		case "storage":
			if s.Specs.StorageGB > 0 {
				c.Score = math.Min(100, float64(s.Specs.StorageGB)/1024*100)
				c.Reason = "演示容量分；不代表硬盘速度或寿命"
				known += weight
			}
		case "monitor":
			if s.Specs.Width > 0 && s.Specs.Height > 0 && s.Specs.RefreshHz > 0 {
				c.Score = math.Min(100, float64(s.Specs.Width*s.Specs.Height)/(2560*1440)*70+float64(s.Specs.RefreshHz)/180*30)
				c.Reason = "演示规格分：像素数 70%、刷新率 30%；不代表画质"
				known += weight
			}
		}
		if c.Reason == "" {
			c.Reason = "缺少可用评分数据，不参与总分"
		}
		weighted += c.Score * weight
		parts = append(parts, c)
		if s.Category == "gpu" {
			all += 5
			brand := ComponentScore{Category: "gpu_brand", Title: s.Brand + " 板卡品牌", BrandTier: c.BrandTier, Reason: "用户设定的演示品牌偏好，权重 5%；不改变 GPU 芯片基准分"}
			switch c.BrandTier {
			case "一线（演示偏好）":
				brand.Score = 100
			case "二线（演示偏好）":
				brand.Score = 70
			}
			if brand.Score > 0 {
				known += 5
				weighted += brand.Score * 5
			} else {
				brand.Reason = "品牌未分层，不参与评分"
			}
			parts = append(parts, brand)
		}
	}
	result := PlanScore{ComparableGroup: group, Method: "v1: 已覆盖品类的加权分；同品类组合内比较；价格和配件规格为模拟数据", Components: parts}
	if known == 0 || amountCents <= 0 {
		return result
	}
	result.Score = math.Round(weighted/known*100) / 100
	result.Coverage = math.Round(known/all*100) / 100
	result.ValuePerThousand = math.Round(result.Score/(float64(amountCents)/100000)*100) / 100
	return result
}
