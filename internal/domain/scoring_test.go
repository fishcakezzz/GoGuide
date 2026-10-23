package domain

import "testing"

func TestScorePlanUsesChipBenchmarkNotBoardBrand(t *testing.T) {
	base := []SKU{
		{Category: "cpu", Title: "酷睿 i5-13400F 处理器"},
		{Category: "gpu", Brand: "七彩虹", Title: "GeForce RTX 4060 8GB 显卡"},
		{Category: "ram", Specs: Specs{MemoryGB: 32, MemorySpeedMT: 6000}},
	}
	cheap := ScorePlan(base, 400000)
	base[1].Brand = "华硕"
	premium := ScorePlan(base, 500000)
	if cheap.Components[1].Benchmark.Mark != premium.Components[1].Benchmark.Mark || cheap.Components[1].Score != premium.Components[1].Score {
		t.Fatalf("board brand changed chip performance: %+v %+v", cheap, premium)
	}
	if premium.Score <= cheap.Score {
		t.Fatalf("brand preference did not affect overall score: %+v %+v", cheap, premium)
	}
	if cheap.ValuePerThousand <= premium.ValuePerThousand {
		t.Fatalf("lower price should have higher value ratio: %+v %+v", cheap, premium)
	}
	if cheap.Components[1].BrandTier == premium.Components[1].BrandTier {
		t.Fatalf("demo brand preferences should be visible")
	}
}

func TestLocalBenchmarkRanks(t *testing.T) {
	a := BenchmarkFor(SKU{Title: "酷睿 i5-13400F"})
	b := BenchmarkFor(SKU{Title: "锐龙 R5 7500F"})
	if a == nil || b == nil || a.LocalCount != 2 || b.LocalRank >= a.LocalRank {
		t.Fatalf("local CPU ordering invalid: %+v %+v", a, b)
	}
}
