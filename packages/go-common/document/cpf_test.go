package document

import "testing"

func TestNormalizeCPF(t *testing.T) {
	for in, want := range map[string]string{"529.982.247-25": "52998224725", "111 444 777 35": "11144477735"} {
		if got, err := NormalizeCPF(in); err != nil || got != want {
			t.Errorf("NormalizeCPF(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "529.982.247-26", "111.111.111-11", "5299822472", "52998224725a", "529/982/247-25"} {
		if _, err := NormalizeCPF(in); err == nil {
			t.Errorf("NormalizeCPF(%q) should fail", in)
		}
	}
}

func TestMaskCPF(t *testing.T) {
	if got := MaskCPF("52998224725"); got != "***.982.247-**" {
		t.Errorf("MaskCPF = %s", got)
	}
}
