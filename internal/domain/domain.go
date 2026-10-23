package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Specs struct {
	MemoryGB             int      `json:"memory_gb,omitempty"`
	MemoryType           string   `json:"memory_type,omitempty"`
	MemorySpeedMT        int      `json:"memory_speed_mt,omitempty"`
	StorageGB            int      `json:"storage_gb,omitempty"`
	StorageInterface     string   `json:"storage_interface,omitempty"`
	StorageInterfaces    []string `json:"storage_interfaces,omitempty"`
	Socket               string   `json:"socket,omitempty"`
	CPUModel             string   `json:"cpu_model,omitempty"`
	SupportedCPUModels   []string `json:"supported_cpu_models,omitempty"`
	MaxPowerW            int      `json:"max_power_w,omitempty"`
	PSUPowerW            int      `json:"psu_power_w,omitempty"`
	SupportedSockets     []string `json:"supported_sockets,omitempty"`
	FormFactor           string   `json:"form_factor,omitempty"`
	SupportedFormFactors []string `json:"supported_form_factors,omitempty"`
	GPUVRAMGB            int      `json:"gpu_vram_gb,omitempty"`
	GPULengthMM          int      `json:"gpu_length_mm,omitempty"`
	MaxGPULengthMM       int      `json:"max_gpu_length_mm,omitempty"`
	CoolerHeightMM       int      `json:"cooler_height_mm,omitempty"`
	MaxCoolerHeightMM    int      `json:"max_cooler_height_mm,omitempty"`
	WeightGrams          int      `json:"weight_grams,omitempty"`
	VideoOut             []string `json:"video_out,omitempty"`
	VideoIn              []string `json:"video_in,omitempty"`
	Width                int      `json:"width,omitempty"`
	Height               int      `json:"height,omitempty"`
	RefreshHz            int      `json:"refresh_hz,omitempty"`
	MaxWidth             int      `json:"max_width,omitempty"`
	MaxHeight            int      `json:"max_height,omitempty"`
	MaxRefreshHz         int      `json:"max_refresh_hz,omitempty"`
	PDOutputW            int      `json:"pd_output_w,omitempty"`
	RequiredPowerW       int      `json:"required_power_w,omitempty"`
	USBCCanCharge        *bool    `json:"usb_c_charging,omitempty"`
}

type SKU struct {
	ID           int64      `json:"id"`
	ProductID    int64      `json:"product_id"`
	Category     string     `json:"category"`
	Brand        string     `json:"brand"`
	Title        string     `json:"title"`
	Label        string     `json:"label"`
	Description  string     `json:"description,omitempty"`
	ImageURL     string     `json:"image_url,omitempty"`
	SourceKind   string     `json:"source_kind,omitempty"`
	SourceURL    string     `json:"source_url,omitempty"`
	Specs        Specs      `json:"specs"`
	PriceCents   int64      `json:"price_cents"`
	PriceVersion int64      `json:"price_version"`
	StockVersion int64      `json:"stock_version"`
	Available    int        `json:"available"`
	Active       bool       `json:"active"`
	ObservedAt   string     `json:"observed_at,omitempty"`
	Benchmark    *Benchmark `json:"benchmark,omitempty"`
	BrandTier    string     `json:"brand_tier,omitempty"`
}

type Intent struct {
	BudgetCents      int64    `json:"budget_cents"`
	Categories       []string `json:"categories"`
	UseCase          string   `json:"use_case"`
	Brand            string   `json:"brand,omitempty"`
	CPUModel         string   `json:"cpu_model,omitempty"`
	GPUModel         string   `json:"gpu_model,omitempty"`
	MinMemoryGB      int      `json:"min_memory_gb,omitempty"`
	MemoryType       string   `json:"memory_type,omitempty"`
	MinMemorySpeedMT int      `json:"min_memory_speed_mt,omitempty"`
	MinStorageGB     int      `json:"min_storage_gb,omitempty"`
	MinGPUVRAMGB     int      `json:"min_gpu_vram_gb,omitempty"`
	MinRefreshHz     int      `json:"min_refresh_hz,omitempty"`
	RequireOneCable  bool     `json:"require_one_cable,omitempty"`
	Hard             []string `json:"hard"`
	Soft             []string `json:"soft"`
	LockedSKUs       []int64  `json:"locked_skus"`
	Questions        int      `json:"questions"`
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

var CategoryNames = map[string]string{
	"laptop": "笔记本", "monitor": "显示器", "dock": "扩展坞",
	"ram": "内存条", "gpu": "显卡", "storage": "硬盘", "cpu": "CPU",
	"cooler": "散热器", "motherboard": "主板", "case": "机箱",
	"keyboard": "键盘", "headset": "耳机",
	"psu": "电源",
}

func ValidCategory(category string) bool {
	_, ok := CategoryNames[category]
	return ok
}

func CheckCompatibility(items []SKU) Compatibility {
	pc := checkPCCompatibility(items)
	if pc.Status != Supported {
		return pc
	}
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
		if len(pc.Reasons) == 0 {
			pc.Reasons = []string{"当前选择没有需要交叉验证的配件"}
		}
		return pc
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
	return Compatibility{Status: Supported, Reasons: append(pc.Reasons, "视频协议、分辨率、刷新率与供电要求均通过结构化数据检查")}
}

func checkPCCompatibility(items []SKU) Compatibility {
	selected := map[string]*SKU{}
	for i := range items {
		selected[items[i].Category] = &items[i]
	}
	reasons := []string{}
	checkEqual := func(a, b, field, left, right string) *Compatibility {
		if a == "" || b == "" {
			return &Compatibility{Status: Unknown, Reasons: []string{left + "与" + right + "缺少" + field + "参数"}}
		}
		if !strings.EqualFold(a, b) {
			return &Compatibility{Status: Unsupported, Reasons: []string{left + "与" + right + "的" + field + "不匹配"}}
		}
		reasons = append(reasons, left+"与"+right+"的"+field+"匹配")
		return nil
	}
	if cpu, board := selected["cpu"], selected["motherboard"]; cpu != nil && board != nil {
		if result := checkEqual(cpu.Specs.Socket, board.Specs.Socket, "CPU 插槽", "CPU", "主板"); result != nil {
			return *result
		}
		if cpu.Specs.CPUModel == "" || len(board.Specs.SupportedCPUModels) == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"缺少 CPU 型号或主板 BIOS 支持列表"}}
		}
		if !containsFold(board.Specs.SupportedCPUModels, cpu.Specs.CPUModel) {
			return Compatibility{Status: Unsupported, Reasons: []string{"主板当前演示 BIOS 支持列表不含 " + cpu.Specs.CPUModel}}
		}
		reasons = append(reasons, "主板演示 BIOS 支持列表包含 "+cpu.Specs.CPUModel)
	}
	if ram, board := selected["ram"], selected["motherboard"]; ram != nil && board != nil {
		if result := checkEqual(ram.Specs.MemoryType, board.Specs.MemoryType, "内存代际", "内存", "主板"); result != nil {
			return *result
		}
	}
	if cooler, cpu := selected["cooler"], selected["cpu"]; cooler != nil && cpu != nil {
		if cpu.Specs.Socket == "" || len(cooler.Specs.SupportedSockets) == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"散热器与 CPU 缺少插槽参数"}}
		}
		if !containsFold(cooler.Specs.SupportedSockets, cpu.Specs.Socket) {
			return Compatibility{Status: Unsupported, Reasons: []string{"散热器不支持 CPU 插槽 " + cpu.Specs.Socket}}
		}
		reasons = append(reasons, "散热器支持 CPU 插槽 "+cpu.Specs.Socket)
	}
	if chassis, board := selected["case"], selected["motherboard"]; chassis != nil && board != nil {
		if board.Specs.FormFactor == "" || len(chassis.Specs.SupportedFormFactors) == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"机箱与主板缺少尺寸参数"}}
		}
		if !containsFold(chassis.Specs.SupportedFormFactors, board.Specs.FormFactor) {
			return Compatibility{Status: Unsupported, Reasons: []string{"机箱不支持主板尺寸 " + board.Specs.FormFactor}}
		}
		reasons = append(reasons, "机箱支持主板尺寸 "+board.Specs.FormFactor)
	}
	if chassis, gpu := selected["case"], selected["gpu"]; chassis != nil && gpu != nil {
		if chassis.Specs.MaxGPULengthMM == 0 || gpu.Specs.GPULengthMM == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"机箱与显卡缺少长度参数"}}
		}
		if gpu.Specs.GPULengthMM > chassis.Specs.MaxGPULengthMM {
			return Compatibility{Status: Unsupported, Reasons: []string{"显卡长度超过机箱限长"}}
		}
		reasons = append(reasons, "显卡长度在机箱限长内")
	}
	if chassis, cooler := selected["case"], selected["cooler"]; chassis != nil && cooler != nil {
		if chassis.Specs.MaxCoolerHeightMM == 0 || cooler.Specs.CoolerHeightMM == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"机箱与散热器缺少高度参数"}}
		}
		if cooler.Specs.CoolerHeightMM > chassis.Specs.MaxCoolerHeightMM {
			return Compatibility{Status: Unsupported, Reasons: []string{"散热器高度超过机箱限高"}}
		}
		reasons = append(reasons, "散热器高度在机箱限高内")
	}
	if board, storage := selected["motherboard"], selected["storage"]; board != nil && storage != nil {
		if storage.Specs.StorageInterface == "" || len(board.Specs.StorageInterfaces) == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"缺少主板存储接口或硬盘接口参数"}}
		}
		if !containsFold(board.Specs.StorageInterfaces, storage.Specs.StorageInterface) {
			return Compatibility{Status: Unsupported, Reasons: []string{"主板不支持硬盘接口 " + storage.Specs.StorageInterface}}
		}
		reasons = append(reasons, "主板支持硬盘接口 "+storage.Specs.StorageInterface)
	}
	if psu := selected["psu"]; psu != nil && (selected["cpu"] != nil || selected["gpu"] != nil) {
		cpu, gpu := selected["cpu"], selected["gpu"]
		if cpu == nil || gpu == nil || cpu.Specs.MaxPowerW == 0 || gpu.Specs.MaxPowerW == 0 || psu.Specs.PSUPowerW == 0 {
			return Compatibility{Status: Unknown, Reasons: []string{"缺少 CPU、显卡或电源功率参数"}}
		}
		// 100 W is an explicit demo allowance for board, RAM, storage and fans.
		required := (cpu.Specs.MaxPowerW + gpu.Specs.MaxPowerW + 100) * 13 / 10
		if psu.Specs.PSUPowerW < required {
			return Compatibility{Status: Unsupported, Reasons: []string{fmt.Sprintf("电源 %dW 低于演示估算需求 %dW", psu.Specs.PSUPowerW, required)}}
		}
		reasons = append(reasons, fmt.Sprintf("电源 %dW 满足演示估算需求 %dW；仍需核对供电线材", psu.Specs.PSUPowerW, required))
	}
	return Compatibility{Status: Supported, Reasons: reasons}
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
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
