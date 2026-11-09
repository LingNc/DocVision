package latex

import "testing"

// T289：文本图不进分组池——它的交付物是提取出来的文字，被并进一组当跟随者
// 就会被跳过作图（自己的文字没人提取，rebuild 只能退回原图）。
func TestMotifDedupeSkipsTextImages(t *testing.T) {
	if motifDedupeEligible(ClassText) {
		t.Fatal("文本图不该参与分组")
	}
	for _, cls := range []string{ClassVector, ClassRaster} {
		if !motifDedupeEligible(cls) {
			t.Fatalf("%s 图应当参与分组", cls)
		}
	}
}

// T289：images 阶段没跑完就不能记 done。跟随者（成品由代表图物化）、被吸收的
// 跨页续片、以及"跑了但没成品"（error，书里按原图交付）都不算"没跑到"；只有
// 压根没跑到的才挡后续阶段——否则一张永远画不出来的图能把整本书永久卡住。
func TestUnfinishedImageKeys(t *testing.T) {
	prog := map[string]*imageProgress{
		"a::done.png":      {Class: ClassVector, Status: ImageStatusDone},
		"a::fallback.png":  {Class: ClassVector, Status: ImageStatusFallback, Kept: true},
		"a::error.png":     {Class: ClassVector, Status: ImageStatusError},
		"a::pending.png":   {Class: ClassVector},
		"a::follower.png":  {Class: ClassVector, Status: "", EquivalentTo: "a::done.png"},
		"a::absorbed.png":  {Class: ClassVector, Status: ImageStatusDone, Absorbed: true},
		"a::unclassed.png": {Status: ""},
	}
	got := unfinishedImageKeys(prog)
	if len(got) != 1 || got[0] != "a::pending.png" {
		t.Fatalf("没跑到的应当只有 pending 那一张（error 按原图交付、不挡后续阶段）: %v", got)
	}
	if errs := errorImageKeys(prog); len(errs) != 1 || errs[0] != "a::error.png" {
		t.Fatalf("没成品的应当只有 error 那一张: %v", errs)
	}
}
