package domain

import "testing"

func TestCheckCompatibility(t *testing.T) {
	yes := true
	laptop := SKU{Category: "laptop", Specs: Specs{VideoOut: []string{"usb-c-dp-1.4"}, MaxWidth: 3840, MaxHeight: 2160, MaxRefreshHz: 60, RequiredPowerW: 65, USBCCanCharge: &yes}}
	monitor := SKU{Category: "monitor", Specs: Specs{VideoIn: []string{"usb-c-dp-1.4"}, Width: 2560, Height: 1440, RefreshHz: 60}}
	dock := SKU{Category: "dock", Specs: Specs{VideoIn: []string{"usb-c-dp-1.4"}, VideoOut: []string{"usb-c-dp-1.4"}, MaxWidth: 3840, MaxHeight: 2160, MaxRefreshHz: 60, PDOutputW: 100}}
	if got := CheckCompatibility([]SKU{laptop, monitor, dock}); got.Status != Supported {
		t.Fatalf("got %+v", got)
	}
	dock.Specs.PDOutputW = 45
	if got := CheckCompatibility([]SKU{laptop, monitor, dock}); got.Status != Unsupported {
		t.Fatalf("got %+v", got)
	}
	dock.Specs.PDOutputW = 0
	if got := CheckCompatibility([]SKU{laptop, monitor, dock}); got.Status != Unknown {
		t.Fatalf("got %+v", got)
	}
	dock.Specs.PDOutputW = 100
	laptop.Specs.MaxWidth = 1920
	if got := CheckCompatibility([]SKU{laptop, monitor, dock}); got.Status != Unsupported {
		t.Fatalf("laptop output limit ignored: %+v", got)
	}
}

func TestPCPartCompatibility(t *testing.T) {
	cpu := SKU{Category: "cpu", Specs: Specs{Socket: "LGA1700", CPUModel: "i5-13400F", MaxPowerW: 148}}
	board := SKU{Category: "motherboard", Specs: Specs{Socket: "LGA1700", MemoryType: "DDR5", FormFactor: "MATX", SupportedCPUModels: []string{"i5-13400F"}, StorageInterfaces: []string{"M.2 NVMe"}}}
	ram := SKU{Category: "ram", Specs: Specs{MemoryType: "DDR5"}}
	cooler := SKU{Category: "cooler", Specs: Specs{SupportedSockets: []string{"LGA1700"}, CoolerHeightMM: 155}}
	gpu := SKU{Category: "gpu", Specs: Specs{GPULengthMM: 320, MaxPowerW: 285}}
	chassis := SKU{Category: "case", Specs: Specs{SupportedFormFactors: []string{"MATX"}, MaxGPULengthMM: 350, MaxCoolerHeightMM: 160}}
	items := []SKU{cpu, board, ram, cooler, gpu, chassis}
	if got := CheckCompatibility(items); got.Status != Supported {
		t.Fatalf("matching build: %+v", got)
	}
	withPSU := append(items, SKU{Category: "psu", Specs: Specs{PSUPowerW: 650}})
	if got := CheckCompatibility(withPSU); got.Status != Unsupported {
		t.Fatalf("undersized PSU: %+v", got)
	}
	withPSU[len(withPSU)-1].Specs.PSUPowerW = 750
	if got := CheckCompatibility(withPSU); got.Status != Supported {
		t.Fatalf("sufficient PSU: %+v", got)
	}
	items[1].Specs.Socket = "AM5"
	if got := CheckCompatibility(items); got.Status != Unsupported {
		t.Fatalf("CPU socket mismatch: %+v", got)
	}
	items[1].Specs.Socket = "LGA1700"
	items[2].Specs.MemoryType = "DDR4"
	if got := CheckCompatibility(items); got.Status != Unsupported {
		t.Fatalf("RAM generation mismatch: %+v", got)
	}
	items[2].Specs.MemoryType = "DDR5"
	items[4].Specs.GPULengthMM = 360
	if got := CheckCompatibility(items); got.Status != Unsupported {
		t.Fatalf("GPU length mismatch: %+v", got)
	}
	items[4].Specs.GPULengthMM = 320
	items[5].Specs.MaxCoolerHeightMM = 0
	if got := CheckCompatibility(items); got.Status != Unknown {
		t.Fatalf("missing case height should stay unknown: %+v", got)
	}
}
