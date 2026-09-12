package sessionview

import "testing"

// TestMeasuredImageTokens 钉住"实测每张多少 token"的算法与它拒绝的数据：
// 只有相邻两次请求都带本地那一半、且请求变大了（新增了图片）才能算；
// 裁剪/压缩导致请求变小、没有新增图片、旧转录没有本地那一半——一律跳过。
func TestMeasuredImageTokens(t *testing.T) {
	sample := func(prompt, text, images int) imageUsageSample {
		return imageUsageSample{prompt: prompt, text: text, images: images, usable: true}
	}

	cases := []struct {
		name       string
		samples    []imageUsageSample
		wantPer    int
		wantSteps  int
		wantImages int
	}{
		{
			name: "一次新增一张：差值即每张",
			// prompt +3000，文本估算 +200 ⇒ 图片占 2800。
			samples:    []imageUsageSample{sample(10000, 9000, 0), sample(13000, 9200, 1)},
			wantPer:    2800,
			wantSteps:  1,
			wantImages: 1,
		},
		{
			name: "一次新增三张：差值按张数摊",
			// prompt +9000，文本 +0 ⇒ 每张 3000。
			samples:    []imageUsageSample{sample(1000, 900, 0), sample(10000, 900, 3)},
			wantPer:    3000,
			wantSteps:  1,
			wantImages: 3,
		},
		{
			name: "多步取中位数（单步噪声不进结果）",
			samples: []imageUsageSample{
				sample(1000, 900, 0),
				sample(4000, 1000, 1),  // 3000
				sample(7000, 1000, 2),  // 3000
				sample(20000, 1000, 3), // 13000（网关回执异常）
			},
			wantPer:    3000,
			wantSteps:  3,
			wantImages: 3,
		},
		{
			name: "裁剪/压缩导致请求变小：跳过，不当成负的图片开销",
			samples: []imageUsageSample{
				sample(50000, 40000, 0),
				sample(12000, 9000, 1), // 修剪之后又加了一张，delta 为负
			},
			wantPer: 0, wantSteps: 0, wantImages: 0,
		},
		{
			name: "没有新增图片的步：跳过",
			samples: []imageUsageSample{
				sample(1000, 900, 1),
				sample(1300, 1200, 1),
			},
			wantPer: 0, wantSteps: 0, wantImages: 0,
		},
		{
			name: "旧转录没有本地那一半：跳过",
			samples: []imageUsageSample{
				{prompt: 1000, text: 0, images: 0},
				{prompt: 4000, text: 0, images: 1},
			},
			wantPer: 0, wantSteps: 0, wantImages: 0,
		},
		{
			name: "文本估算涨得比 prompt 还多（估算偏差）：跳过",
			samples: []imageUsageSample{
				sample(1000, 900, 0),
				sample(1100, 5000, 1),
			},
			wantPer: 0, wantSteps: 0, wantImages: 0,
		},
		{
			name: "两次请求之间隔着裁剪、只有一步可用：只用那一步",
			samples: []imageUsageSample{
				sample(80000, 70000, 1),
				sample(9000, 8000, 0), // 压缩后变小：跳过
				sample(12000, 8300, 1),
			},
			wantPer:    2700,
			wantSteps:  1,
			wantImages: 1,
		},
		{
			name:    "没有相邻两条可用样本：给不出实测值",
			samples: []imageUsageSample{sample(1000, 900, 0)},
			wantPer: 0, wantSteps: 0, wantImages: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			per, steps, images := measuredImageTokens(c.samples)
			if per != c.wantPer || steps != c.wantSteps || images != c.wantImages {
				t.Fatalf("measuredImageTokens = (%d, %d, %d)，期望 (%d, %d, %d)",
					per, steps, images, c.wantPer, c.wantSteps, c.wantImages)
			}
		})
	}
}

// TestMeasuredImageTokensFromTranscript 用一份真实形状的转录走一遍：
// 用量行的 image_count/text_tokens 落到会话信息里，页面直接可用。
func TestMeasuredImageTokensFromTranscript(t *testing.T) {
	root := t.TempDir()
	writeFile(t, jsonl(root, "proj", "work", "sessions", "meas_02.jsonl"), transcript(
		`{"t":"meta","kind":"system","session_label":"convert:01","model":"wire-A","text":"提示词"}`,
		`{"t":"usage","model":"wire-A","round":1,"prompt_tokens":10000,"text_tokens":9000,"image_count":0}`,
		`{"t":"usage","model":"wire-A","round":2,"prompt_tokens":13000,"text_tokens":9200,"image_count":1}`,
		`{"t":"usage","model":"wire-A","round":3,"prompt_tokens":16000,"text_tokens":9400,"image_count":2}`,
	))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Estimate == nil {
		t.Fatalf("没有拿到会话估算: %+v", sessions)
	}
	est := sessions[0].Estimate
	// 两步：(3000−200)/1 = 2800 与 (3000−200)/1 = 2800 → 中位数 2800。
	if est.MeasuredPerImage != 2800 || est.MeasuredSamples != 2 || est.MeasuredImages != 2 {
		t.Fatalf("实测值 = %d（%d 步 / %d 张），期望 2800 / 2 / 2",
			est.MeasuredPerImage, est.MeasuredSamples, est.MeasuredImages)
	}
	if est.Model != "wire-A" || est.Rule == "" {
		t.Fatalf("会话的规则没带上: %+v", est)
	}
}
