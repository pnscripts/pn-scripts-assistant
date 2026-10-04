package catalogue

import (
	"reflect"
	"testing"
)

func TestEverySizeTheLibraryPublishesIsASize(t *testing.T) {
	page := `
<li><a href="/library/all-minilm">
  <p class="max-w-lg">Embedding models.</p>
  <span class="text-indigo-600">embedding</span>
  <span class="text-indigo-600">22m</span><span class="text-indigo-600">33m</span>
</a></li>
<li><a href="/library/dolphin-mixtral">
  <p class="max-w-lg">Uncensored.</p>
  <span class="text-indigo-600">8x7b</span><span class="text-indigo-600">8x22b</span>
</a></li>
<li><a href="/library/gemma3n">
  <p class="max-w-lg">Everyday devices.</p>
  <span class="text-indigo-600">e2b</span><span class="text-indigo-600">e4b</span>
</a></li>
<li><a href="/library/qwen3">
  <p class="max-w-lg">Qwen.</p>
  <span class="text-indigo-600">tools</span><span class="text-indigo-600">thinking</span>
  <span class="text-indigo-600">0.6b</span><span class="text-indigo-600">8b</span>
</a></li>`

	got := map[string]Model{}

	for _, m := range parse(page) {
		got[m.Name] = m
	}

	for name, want := range map[string]struct{ can, sizes []string }{
		"all-minilm":      {[]string{"embedding"}, []string{"22m", "33m"}},
		"dolphin-mixtral": {nil, []string{"8x7b", "8x22b"}},
		"gemma3n":         {nil, []string{"e2b", "e4b"}},
		"qwen3":           {[]string{"tools", "thinking"}, []string{"0.6b", "8b"}},
	} {
		m := got[name]

		if !reflect.DeepEqual(m.Can, want.can) || !reflect.DeepEqual(m.Sizes, want.sizes) {
			t.Errorf("%s: can %v, sizes %v; want can %v, sizes %v", name, m.Can, m.Sizes, want.can, want.sizes)
		}
	}
}

func TestBillionsReadsEveryKindOfSize(t *testing.T) {
	for size, want := range map[string]float64{
		"8b":     8,
		"0.6b":   0.6,
		"1.5B":   1.5,
		"270m":   0.27,
		"1t":     1000,
		"8x7b":   56,
		"8x22b":  176,
		"e2b":    0, // effective, not what is loaded
		"":       0,
		"latest": 0,
	} {
		if got := Billions(size); got < want-1e-9 || got > want+1e-9 {
			t.Errorf("Billions(%q) = %v, want %v", size, got, want)
		}
	}

	if MemoryFor("e4b") != 0 {
		t.Error("an effective size was given a memory estimate it cannot have")
	}

	if got := MemoryFor("8b"); got < 5 || got > 7 {
		t.Errorf("8b is estimated at %.1fGB", got)
	}
}
