package quality

import (
	"strings"
)

// Profile 描述一个物种可确定性执行的检测方案。
type Profile struct {
	Code               string   `json:"code"`
	SpeciesAliases     []string `json:"species_aliases"`
	MinimumSamples     int      `json:"minimum_samples"`
	MinimumReplicates  int      `json:"minimum_replicates"`
	MinimumGermination float64  `json:"minimum_germination"`
	MinimumPurity      float64  `json:"minimum_purity"`
	MaximumMoisture    float64  `json:"maximum_moisture"`
	AllowedMethods     []string `json:"allowed_methods"`
}

var profiles = []Profile{
	{Code: "WHEAT-STD", SpeciesAliases: []string{"小麦", "普通小麦", "triticum aestivum"}, MinimumSamples: 400, MinimumReplicates: 4, MinimumGermination: 85, MinimumPurity: 98, MaximumMoisture: 13, AllowedMethods: []string{"GB/T3543", "ISTA"}},
	{Code: "RICE-STD", SpeciesAliases: []string{"水稻", "稻", "oryza sativa"}, MinimumSamples: 400, MinimumReplicates: 4, MinimumGermination: 80, MinimumPurity: 98, MaximumMoisture: 13.5, AllowedMethods: []string{"GB/T3543", "ISTA"}},
	{Code: "MAIZE-STD", SpeciesAliases: []string{"玉米", "zea mays"}, MinimumSamples: 400, MinimumReplicates: 4, MinimumGermination: 85, MinimumPurity: 97, MaximumMoisture: 13, AllowedMethods: []string{"GB/T3543", "ISTA"}},
	{Code: "SOYBEAN-STD", SpeciesAliases: []string{"大豆", "glycine max"}, MinimumSamples: 400, MinimumReplicates: 4, MinimumGermination: 80, MinimumPurity: 98, MaximumMoisture: 12, AllowedMethods: []string{"GB/T3543", "ISTA"}},
}

// ProfileForSpecies 返回按规范化别名匹配的方案。
func ProfileForSpecies(species string) (Profile, bool) {
	wanted := strings.ToLower(strings.TrimSpace(species))
	for _, profile := range profiles {
		for _, alias := range profile.SpeciesAliases {
			if wanted == strings.ToLower(alias) {
				return profile, true
			}
		}
	}
	return Profile{}, false
}

// Profiles 返回只读副本供页面展示。
func Profiles() []Profile {
	result := make([]Profile, len(profiles))
	copy(result, profiles)
	return result
}
