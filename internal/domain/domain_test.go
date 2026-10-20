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
