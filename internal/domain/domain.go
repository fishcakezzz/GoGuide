package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Specs struct {
	MemoryGB       int      `json:"memory_gb,omitempty"`
	StorageGB      int      `json:"storage_gb,omitempty"`
	WeightGrams    int      `json:"weight_grams,omitempty"`
	VideoOut       []string `json:"video_out,omitempty"`
	VideoIn        []string `json:"video_in,omitempty"`
	Width          int      `json:"width,omitempty"`
	Height         int      `json:"height,omitempty"`
	RefreshHz      int      `json:"refresh_hz,omitempty"`
	MaxWidth       int      `json:"max_width,omitempty"`
	MaxHeight      int      `json:"max_height,omitempty"`
	MaxRefreshHz   int      `json:"max_refresh_hz,omitempty"`
	PDOutputW      int      `json:"pd_output_w,omitempty"`
	RequiredPowerW int      `json:"required_power_w,omitempty"`
	USBCCanCharge  *bool    `json:"usb_c_charging,omitempty"`
}

type SKU struct {
	ID           int64  `json:"id"`
	ProductID    int64  `json:"product_id"`
	Category     string `json:"category"`
	Brand        string `json:"brand"`
	Title        string `json:"title"`
	Label        string `json:"label"`
	Description  string `json:"description,omitempty"`
	ImageURL     string `json:"image_url,omitempty"`
	SourceKind   string `json:"source_kind,omitempty"`
	SourceURL    string `json:"source_url,omitempty"`
	Specs        Specs  `json:"specs"`
	PriceCents   int64  `json:"price_cents"`
	PriceVersion int64  `json:"price_version"`
	Available    int    `json:"available"`
	Active       bool   `json:"active"`
	ObservedAt   string `json:"observed_at,omitempty"`
}

type Intent struct {
	BudgetCents     int64    `json:"budget_cents"`
	Categories      []string `json:"categories"`
	UseCase         string   `json:"use_case"`
	Brand           string   `json:"brand,omitempty"`
	MinMemoryGB     int      `json:"min_memory_gb,omitempty"`
	RequireOneCable bool     `json:"require_one_cable,omitempty"`
	Hard            []string `json:"hard"`
	Soft            []string `json:"soft"`
	LockedSKUs      []int64  `json:"locked_skus"`
	Questions       int      `json:"questions"`
}

type Compatibility struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons"`
}

const (
	Supported   = "SUPPORTED"
	Unsupported = "UNSUPPORTED"
	Unknown     = "UNKNOWN"
)

func CheckCompatibility(items []SKU) Compatibility {
	var laptop, monitor, dock *SKU
	for i := range items {
		switch items[i].Category {
		case "laptop":
			laptop = &items[i]
		case "monitor":
			monitor = &items[i]
		case "dock":
			dock = &items[i]
		}
	}
	if laptop == nil || monitor == nil {
		return Compatibility{Status: Supported, Reasons: []string{"无需检查笔记本与显示器连接"}}
	}
	if len(laptop.Specs.VideoOut) == 0 || len(monitor.Specs.VideoIn) == 0 {
		return Compatibility{Status: Unknown, Reasons: []string{"缺少视频接口协议参数"}}
	}
	output := laptop.Specs.VideoOut
	if dock != nil {
		if laptop.Specs.USBCCanCharge == nil {
			return Compatibility{Status: Unknown, Reasons: []string{"笔记本 USB-C 充电支持未知"}}
		}
		if !*laptop.Specs.USBCCanCharge {
			return Compatibility{Status: Unsupported, Reasons: []string{"笔记本不支持 USB-C 充电，无法一线连接"}}
		}
		if len(dock.Specs.VideoIn) == 0 || len(dock.Specs.VideoOut) == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"扩展坞缺少上行或下行视频协议参数"}}
		}
		if !overlap(output, dock.Specs.VideoIn) {
			return Compatibility{Status: Unsupported, Reasons: []string{"笔记本与扩展坞的视频协议不匹配"}}
		}
		output = dock.Specs.VideoOut
		if laptop.Specs.RequiredPowerW == 0 || dock.Specs.PDOutputW == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"缺少笔记本所需功率或扩展坞供电功率"}}
		}
		if dock.Specs.PDOutputW < laptop.Specs.RequiredPowerW {
			return Compatibility{Status: Unsupported, Reasons: []string{fmt.Sprintf("扩展坞供电 %dW 低于笔记本所需 %dW", dock.Specs.PDOutputW, laptop.Specs.RequiredPowerW)}}
		}
	}
	if !overlap(output, monitor.Specs.VideoIn) {
		return Compatibility{Status: Unsupported, Reasons: []string{"视频输出与显示器输入协议不匹配"}}
	}
	capability := laptop.Specs
	if dock != nil {
		capability = dock.Specs
	}
	if monitor.Specs.Width == 0 || monitor.Specs.Height == 0 || monitor.Specs.RefreshHz == 0 || capability.MaxWidth == 0 || capability.MaxHeight == 0 || capability.MaxRefreshHz == 0 {
		return Compatibility{Status: Unknown, Reasons: []string{"缺少分辨率或刷新率能力参数"}}
	}
	if monitor.Specs.Width > capability.MaxWidth || monitor.Specs.Height > capability.MaxHeight || monitor.Specs.RefreshHz > capability.MaxRefreshHz {
		return Compatibility{Status: Unsupported, Reasons: []string{"视频输出能力不足以驱动该分辨率或刷新率"}}
	}
	if dock != nil {
		if laptop.Specs.MaxWidth == 0 || laptop.Specs.MaxHeight == 0 || laptop.Specs.MaxRefreshHz == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"笔记本视频输出能力参数缺失"}}
		}
		if monitor.Specs.Width > laptop.Specs.MaxWidth || monitor.Specs.Height > laptop.Specs.MaxHeight || monitor.Specs.RefreshHz > laptop.Specs.MaxRefreshHz {
			return Compatibility{Status: Unsupported, Reasons: []string{"笔记本输出能力不足以驱动显示器"}}
		}
	}
	return Compatibility{Status: Supported, Reasons: []string{"视频协议、分辨率、刷新率与供电要求均通过结构化数据检查"}}
}

func overlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(x, y) {
				return true
			}
		}
	}
	return false
}

func Encode(v any) []byte { b, _ := json.Marshal(v); return b }
