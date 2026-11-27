// Package graphics 生成方舟客户端的画质设置：ASE 写进 BaseDeviceProfiles.ini，ASA 生成控制台命令。
//
// 参数取自玩家圈子里公开流传的画质配置（ARKCademy 的 BaseDeviceProfiles.ini 预设、
// Wikily 的 ASA INI 生成器等），只改本机的画面，不碰游戏进程。
package graphics

// Game 是游戏版本。
type Game string

const (
	ASE Game = "ase" // ARK: Survival Evolved
	ASA Game = "asa" // ARK: Survival Ascended
)

// 选项分组。
const (
	GroupPerf   = "perf"   // 性能：提高帧数，基本不影响看到的东西
	GroupCombat = "combat" // 战斗画质：去掉遮挡视野的效果
)

// Option 是一项画质设置。ASE 的 Lines 是 ini 里 +CVars= 后面的内容，ASA 的 Lines 是控制台命令。
type Option struct {
	ID         string   `json:"id"`
	Group      string   `json:"group"`
	Name       string   `json:"name"`
	Desc       string   `json:"desc"`
	SideEffect string   `json:"side_effect"` // 副作用，没有为空
	Lines      []string `json:"lines"`
	// Console 是 ASE 在游戏控制台里的写法（重启游戏恢复默认），空表示只能写进 ini。
	// 只读参数（r.DefaultFeature.*）和 ShowFlag.* 不能在运行时改，只在 ini 里。ASA 不用这个字段。
	Console []string `json:"console"`
	// ConsoleNote 说明控制台写法和 ini 的差别，没有差别为空。
	ConsoleNote string `json:"console_note"`
}

// ConsoleLines 返回选项在控制台里执行的命令：ASA 是 Lines，ASE 是 Console。
func (c Catalog) ConsoleLines(o Option) []string {
	if c.Game == ASA {
		return o.Lines
	}
	return o.Console
}

// Preset 是一键档位，按顺序启用 Options 里的选项。
type Preset struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Desc    string   `json:"desc"`
	Options []string `json:"options"`
}

// Catalog 是一个游戏版本的全部选项和档位。
type Catalog struct {
	Game    Game     `json:"game"`
	Options []Option `json:"options"`
	Presets []Preset `json:"presets"`
}

// Lookup 按 ID 找选项。
func (c Catalog) Lookup(id string) (Option, bool) {
	for _, o := range c.Options {
		if o.ID == id {
			return o, true
		}
	}
	return Option{}, false
}

// Ordered 返回 ids 里认得的选项，按目录里的顺序排列、去重。
func (c Catalog) Ordered(ids []string) []Option {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []Option
	for _, o := range c.Options {
		if want[o.ID] {
			out = append(out, o)
		}
	}
	return out
}

// For 返回游戏版本的目录。
func For(g Game) Catalog {
	if g == ASA {
		return asaCatalog
	}
	return aseCatalog
}

var aseCatalog = Catalog{
	Game: ASE,
	Options: []Option{
		{
			ID: "post", Group: GroupPerf, Name: "关闭后处理特效",
			Desc:        "关掉泛光、景深、动态模糊、镜头光晕、色差和自动曝光，画面更干净，帧数更高",
			Lines:       []string{"r.BloomQuality=0", "r.DepthOfFieldQuality=0", "r.MotionBlurQuality=0", "r.LensFlareQuality=0", "r.SceneColorFringeQuality=0", "r.SceneColorFringe.Max=0", "r.TonemapperQuality=0", "r.DefaultFeature.Bloom=False", "r.DefaultFeature.AutoExposure=False", "r.DefaultFeature.MotionBlur=False", "r.DefaultFeature.LensFlare=False", "r.LightFunctionQuality=0", "r.SSS.Scale=0", "r.SSS.SampleSet=0"},
			Console:     []string{"r.BloomQuality 0", "r.DepthOfFieldQuality 0", "r.MotionBlurQuality 0", "r.LensFlareQuality 0", "r.SceneColorFringeQuality 0", "r.TonemapperQuality 0", "r.LightFunctionQuality 0", "r.SSS.Scale 0"},
			ConsoleNote: "自动曝光等启动时才读取的参数只能写进 ini",
		},
		{
			ID: "shadows", Group: GroupPerf, Name: "去除阴影",
			Desc:       "关掉动态阴影，低配电脑帧数提升最明显",
			SideEffect: "物体没有影子，立体感变差",
			Lines:      []string{"r.ShadowQuality=0", "r.Shadow.CSM.MaxCascades=1", "r.Shadow.MaxResolution=2", "r.Shadow.MinResolution=2", "r.Shadow.RadiusThreshold=0.1", "r.Shadow.DistanceScale=0.1", "r.Shadow.CSM.TransitionScale=0"},
			Console:    []string{"r.ShadowQuality 0", "r.Shadow.CSM.MaxCascades 1", "r.Shadow.MaxResolution 2", "r.Shadow.DistanceScale 0.1"},
		},
		{
			ID: "reflections", Group: GroupPerf, Name: "去除反射和环境光遮蔽",
			Desc:    "关掉屏幕空间反射、反射环境和环境光遮蔽（AO）",
			Lines:   []string{"r.SSR.Quality=0", "r.ReflectionEnvironment=0", "r.AmbientOcclusionLevels=0", "r.DefaultFeature.AmbientOcclusion=False", "r.RefractionQuality=0"},
			Console: []string{"r.SSR.Quality 0", "r.ReflectionEnvironment 0", "r.AmbientOcclusionLevels 0", "r.RefractionQuality 0"},
		},
		{
			ID: "sky", Group: GroupCombat, Name: "去除云层和天空",
			Desc:        "去掉云层和大气散射，天空变暗，远处不再发白",
			SideEffect:  "天空是黑的",
			Lines:       []string{"r.TrueSkyQuality=0", "r.Atmosphere=0", "ShowFlag.Atmosphere=0"},
			Console:     []string{"r.TrueSkyQuality 0", "r.Atmosphere 0"},
			ConsoleNote: "ShowFlag 部分只能写进 ini，控制台效果可能弱一些",
		},
		{
			ID: "fog", Group: GroupCombat, Name: "除雾",
			Desc:  "去掉大气雾，远处看得更清楚",
			Lines: []string{"ShowFlag.AtmosphericFog=0", "FogDensity=0.0"},
		},
		{
			ID: "lightshafts", Group: GroupCombat, Name: "去除光柱",
			Desc:    "去掉阳光的光柱（丁达尔效应），逆光时不再晃眼",
			Lines:   []string{"r.LightShafts=0", "r.LightShaftQuality=0"},
			Console: []string{"r.LightShafts 0", "r.LightShaftQuality 0"},
		},
		{
			ID: "underwater", Group: GroupCombat, Name: "去除水下致盲",
			Desc:        "水下和洞穴里看得清楚，不用穿潜水服",
			SideEffect:  "泰克头盔、雪鸮、血尾虫的透视 / 扫描能力不能用",
			Lines:       []string{"r.CustomDepth=0"},
			Console:     []string{"r.CustomDepth 0"},
			ConsoleNote: "控制台里改可能要重新进游戏才生效，不行就写进 ini",
		},
		{
			ID: "foliage", Group: GroupCombat, Name: "去除地面植被",
			Desc:        "去掉草和地面装饰物，地上的东西更容易看到。实验性，部分版本可能无效",
			SideEffect:  "地面光秃秃的",
			Lines:       []string{"foliage.UseOcclusionType=0", "grass.DensityScale=0", "foliage.DensityScale=0"},
			Console:     []string{"grass.DensityScale 0", "foliage.DensityScale 0"},
			ConsoleNote: "可能无效",
		},
		{
			ID: "translucency", Group: GroupCombat, Name: "去除水面和树叶",
			Desc:       "去掉水面和部分半透明的树叶，相当于社区 Hard 档。实验性",
			SideEffect: "看不到建筑摆放预览、畸变金属和硅珍珠，泰克武器和泰克复制器的光效消失，很多物品变黑",
			Lines:      []string{"ShowFlag.SeparateTranslucency=0", "ShowFlag.Translucency=0"},
		},
	},
	Presets: []Preset{
		{ID: "perf", Name: "性能", Desc: "只提帧数，不改能看到的东西", Options: []string{"post", "shadows", "reflections"}},
		{ID: "soft", Name: "柔和战斗", Desc: "去云、去雾、去光柱，保留泰克头盔，水下照常", Options: []string{"post", "shadows", "reflections", "sky", "fog", "lightshafts"}},
		{ID: "pvp", Name: "战斗", Desc: "在柔和战斗的基础上水下和洞穴变清楚，泰克头盔失效", Options: []string{"post", "shadows", "reflections", "sky", "fog", "lightshafts", "underwater", "foliage"}},
		{ID: "hard", Name: "极限", Desc: "再去掉水面和树叶，副作用很多", Options: []string{"post", "shadows", "reflections", "sky", "fog", "lightshafts", "underwater", "foliage", "translucency"}},
	},
}

var asaCatalog = Catalog{
	Game: ASA,
	Options: []Option{
		{
			ID: "fps", Group: GroupPerf, Name: "解除帧数上限",
			Desc:  "关掉垂直同步，不限帧数",
			Lines: []string{"t.MaxFPS 0", "r.vsync 0"},
		},
		{
			ID: "post", Group: GroupPerf, Name: "关闭后处理特效",
			Desc:  "关掉泛光、景深、动态模糊、镜头光晕和自动曝光",
			Lines: []string{"r.BloomQuality 0", "r.DepthOfFieldQuality 0", "r.MotionBlur.Amount 0", "r.EyeAdaptationQuality 0", "r.LensFlareQuality 0"},
		},
		{
			ID: "shadows", Group: GroupPerf, Name: "去除阴影",
			Desc:       "关掉虚拟阴影、级联阴影和接触阴影，帧数提升明显",
			SideEffect: "物体没有影子",
			Lines:      []string{"r.ShadowQuality 0", "r.Shadow.Virtual.Enable 0", "r.Shadow.CSM.MaxCascades 0", "r.ContactShadows 0", "r.DistanceFieldShadowing 0"},
		},
		{
			ID: "lumen", Group: GroupPerf, Name: "关闭全局光照和反射",
			Desc:       "关掉 Lumen 全局光照和反射，帧数提升最多的一项",
			SideEffect: "室内和阴影处变暗，可以配合伽马调亮",
			Lines:      []string{"r.Lumen.DiffuseIndirect.Allow 0", "r.Lumen.Reflections.Allow 0", "r.DynamicGlobalIlluminationMethod 0", "sg.GlobalIlluminationQuality 0", "r.SSR.Quality 0"},
		},
		{
			ID: "debris", Group: GroupPerf, Name: "去除爆炸碎块",
			Desc:  "炸家时不生成建筑碎块，打团不掉帧",
			Lines: []string{"ark.MaxActiveDestroyedMeshGeoCollectionCount 0"},
		},
		{
			ID: "textures", Group: GroupPerf, Name: "低材质贴图",
			Desc:       "降低贴图精度，省显存",
			SideEffect: "贴图模糊",
			Lines:      []string{"sg.TextureQuality 0", "r.MipMapLODBias 1"},
		},
		{
			ID: "clouds", Group: GroupCombat, Name: "去除云层",
			Desc:  "去掉体积云",
			Lines: []string{"r.VolumetricCloud 0"},
		},
		{
			ID: "fog", Group: GroupCombat, Name: "除雾",
			Desc:  "去掉普通雾和体积雾，沼泽、红木林里也看得远",
			Lines: []string{"r.Fog 0", "r.VolumetricFog 0"},
		},
		{
			ID: "sky", Group: GroupCombat, Name: "去除天空大气",
			Desc:       "去掉大气散射，远处不再发白",
			SideEffect: "天空变暗",
			Lines:      []string{"r.SkyAtmosphere 0"},
		},
		{
			ID: "lightshafts", Group: GroupCombat, Name: "去除光柱",
			Desc:  "去掉阳光的光柱，逆光时不再晃眼",
			Lines: []string{"r.LightShaftQuality 0"},
		},
		{
			ID: "underwater", Group: GroupCombat, Name: "去除水下致盲",
			Desc:       "关掉后处理材质和折射，水下、洞穴里看得清楚",
			SideEffect: "依赖后处理的视觉效果（如泰克头盔的视野效果）可能失效",
			Lines:      []string{"r.PostProcessing.DisableMaterials 1", "r.RefractionQuality 0"},
		},
		{
			ID: "water", Group: GroupCombat, Name: "去除水面反光",
			Desc:  "去掉水面反射",
			Lines: []string{"r.Water.SingleLayer.Reflection 0"},
		},
		{
			ID: "grass", Group: GroupCombat, Name: "去除草和地面杂物",
			Desc:  "去掉草、小石子和地面装饰，地上的东西和贴地的生物更显眼",
			Lines: []string{"grass.Enable 0", "grass.SizeScale 0", "grass.DensityScale 0", "sg.FoliageQuality 0"},
		},
		{
			ID: "particles", Group: GroupCombat, Name: "去除粒子特效",
			Desc:       "去掉火焰、火花、烟雾等粒子",
			SideEffect: "部分武器和技能的特效看不到",
			Lines:      []string{"FX.MaxCPUParticlesPerEmitter 0", "fx.EnableNiagaraSpriteRendering 0"},
		},
		{
			ID: "water_surface", Group: GroupCombat, Name: "去除水面",
			Desc:       "水面变成透明。部分服务器上无效",
			SideEffect: "看不出水的边界",
			Lines:      []string{"r.Water.SingleLayer 0"},
		},
		{
			ID: "terrain", Group: GroupCombat, Name: "纯地形模式",
			Desc:       "隐藏树木、石头和地面植被，只剩地形和建筑。这几条是开关命令，再执行一次就恢复",
			SideEffect: "采集资源也看不到",
			Lines:      []string{"show InstancedFoliage", "show InstancedGrass", "show InstancedStaticMeshes"},
		},
	},
	Presets: []Preset{
		{ID: "quality", Name: "质量优化", Desc: "去云、除雾、去水下致盲，其他不变", Options: []string{"clouds", "fog", "underwater"}},
		{ID: "balanced", Name: "平衡优化", Desc: "质量优化 + 去草和地面杂物", Options: []string{"clouds", "fog", "underwater", "grass"}},
		{ID: "perf", Name: "性能优化", Desc: "平衡优化 + 去阴影、关全局光照、关后处理", Options: []string{"fps", "post", "shadows", "lumen", "debris", "clouds", "fog", "underwater", "grass"}},
		{ID: "pvp", Name: "战斗", Desc: "性能优化 + 去天空、光柱、水面反光、粒子，低材质", Options: []string{"fps", "post", "shadows", "lumen", "debris", "textures", "clouds", "fog", "sky", "lightshafts", "underwater", "water", "grass", "particles"}},
	},
}
