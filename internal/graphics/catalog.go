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
	Tip        string   `json:"tip"`         // 选了这一项时要玩家自己做的事（例如加启动项），没有为空
	Lines      []string `json:"lines"`
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

// ASE 的 ShowFlag 名字和实际效果对不上（例如 VisualizeLPV 关的是天空、GameplayDebug 去的是水面），
// 这里每一项都按社区实测的对应关系写：入梦方舟小工具公开的单项写法、B 站棱煋的「战斗画质进阶版」、
// ARKCademy 各档预设的对比。不要按名字猜着加。
var aseCatalog = Catalog{
	Game: ASE,
	Options: []Option{
		{
			ID: "post", Group: GroupPerf, Name: "关闭后处理特效",
			Desc:  "关掉泛光、景深、动态模糊、镜头光晕、色差和自动曝光，画面更干净，帧数更高",
			Lines: []string{"r.BloomQuality=0", "r.DepthOfFieldQuality=0", "r.MotionBlurQuality=0", "r.LensFlareQuality=0", "r.SceneColorFringeQuality=0", "r.TonemapperQuality=0", "r.DefaultFeature.Bloom=False", "r.DefaultFeature.AutoExposure=False", "r.DefaultFeature.MotionBlur=False", "r.DefaultFeature.LensFlare=False", "r.LightFunctionQuality=0", "r.SSS.Scale=0", "r.SSS.SampleSet=0"},
		},
		{
			ID: "shadows", Group: GroupPerf, Name: "去除阴影",
			Desc:       "关掉动态阴影，低配电脑帧数提升最明显",
			SideEffect: "物体没有影子，立体感变差",
			Lines:      []string{"r.ShadowQuality=0", "r.Shadow.CSM.MaxCascades=1", "r.Shadow.MaxResolution=2", "r.Shadow.MinResolution=2", "r.Shadow.RadiusThreshold=0.1", "r.Shadow.DistanceScale=0.1", "r.Shadow.CSM.TransitionScale=0"},
		},
		{
			ID: "reflections", Group: GroupPerf, Name: "去除反射和环境光遮蔽",
			Desc:  "关掉屏幕空间反射、反射环境和环境光遮蔽（AO）",
			Lines: []string{"r.SSR.Quality=0", "r.ReflectionEnvironment=0", "r.AmbientOcclusionLevels=0", "r.DefaultFeature.AmbientOcclusion=False"},
		},
		{
			ID: "sky", Group: GroupCombat, Name: "关闭天空和云层",
			Desc:       "去掉天空和云层，抬头看飞行坐骑不再被云挡住、不再晃眼",
			SideEffect: "天空是黑的",
			Lines:      []string{"ShowFlag.VisualizeLPV=0", "r.TrueSkyQuality=0"},
		},
		{
			ID: "fog", Group: GroupCombat, Name: "除雾",
			Desc:  "去掉大气雾和远处的雾气、景深，沼泽和远处看得更清楚",
			Lines: []string{"ShowFlag.AtmosphericFog=0", "ShowFlag.Atmosphere=0", "r.Atmosphere=0", "FogDensity=0.0", "ShowFlag.StaticMeshes=0", "ShowFlag.DepthOfField=0"},
		},
		{
			ID: "lightshafts", Group: GroupCombat, Name: "去除光柱",
			Desc:  "去掉阳光的光柱（丁达尔效应），逆光时不再晃眼",
			Lines: []string{"r.LightShafts=0", "r.LightShaftQuality=0"},
		},
		{
			ID: "underwater", Group: GroupCombat, Name: "去除水下致盲",
			Desc:       "去掉水下的模糊和变暗，不穿潜水服也看得清楚",
			SideEffect: "泰克头盔的扫描类能力可能不能用",
			Lines:      []string{"ShowFlag.Materials=0", "r.SceneColorFringe.Max=0"},
		},
		{
			ID: "refraction", Group: GroupCombat, Name: "关闭折射",
			Desc:  "去掉水下、火焰和热浪的画面扭曲",
			Lines: []string{"ShowFlag.Refraction=0", "r.RefractionQuality=0"},
		},
		{
			ID: "leaves", Group: GroupCombat, Name: "去除树叶和灌木丛",
			Desc:       "树只剩树干，灌木丛消失，躲在树丛里的人和恐龙藏不住",
			SideEffect: "地上的部分资源显示不出来",
			Lines:      []string{"foliage.UseOcclusionType=0"},
		},
		{
			ID: "fullbright", Group: GroupCombat, Name: "地图表面覆盖光",
			Desc:       "去除黑暗，夜里和洞穴里像白天一样亮，帧数也会大幅提升",
			SideEffect: "雪山会飘雪花；部分地图（如瓦尔盖罗）屏幕会半黑半白，要加启动项",
			Tip:        "开了「地图表面覆盖光」后，部分地图屏幕会半黑半白：在 Steam 库里右键 ARK: Survival Evolved →「属性」→「启动选项」加上 -sm4，再启动游戏",
			Lines:      []string{"ShowFlag.LightComplexity=0"},
		},
		{
			ID: "specular", Group: GroupCombat, Name: "覆盖反射效果",
			Desc:  "去掉物体表面的高光反光，金属和水面不再刺眼",
			Lines: []string{"ShowFlag.OverrideDiffuseAndSpecular=0"},
		},
		{
			ID: "water", Group: GroupCombat, Name: "去除水面",
			Desc:       "水面消失，水里的东西直接看得到",
			SideEffect: "看不出水的边界；粒子特效也一起去掉",
			Lines:      []string{"ShowFlag.GameplayDebug=0"},
		},
		{
			ID: "particles", Group: GroupCombat, Name: "关闭粒子特效",
			Desc:       "去掉火焰、爆炸、烟雾和畸变的光点，炸家和打团时不挡视线",
			SideEffect: "部分武器和技能的特效看不到",
			Lines:      []string{"ShowFlag.Navigation=0"},
		},
		{
			ID: "terrain", Group: GroupCombat, Name: "纯地形模式",
			Desc:       "树、石头和地面植被全部隐藏，只剩地形、建筑和生物",
			SideEffect: "所有采集资源都看不到，采集时要关掉",
			Lines:      []string{"foliage.CullAll=1"},
		},
	},
	Presets: []Preset{
		{ID: "perf", Name: "性能", Desc: "只提帧数，不改能看到的东西", Options: []string{"post", "shadows", "reflections"}},
		{ID: "soft", Name: "柔和战斗", Desc: "去天空、除雾、去光柱，泰克头盔和水下照常", Options: []string{"post", "shadows", "reflections", "sky", "fog", "lightshafts"}},
		{ID: "pvp", Name: "战斗", Desc: "柔和战斗 + 去水下致盲、关折射、去树叶", Options: []string{"post", "shadows", "reflections", "sky", "fog", "lightshafts", "underwater", "refraction", "leaves"}},
		{ID: "hard", Name: "极限", Desc: "战斗 + 覆盖光、去反光、去水面、关粒子，副作用多", Options: []string{"post", "shadows", "reflections", "sky", "fog", "lightshafts", "underwater", "refraction", "leaves", "fullbright", "specular", "water", "particles"}},
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
			ID: "resolution", Group: GroupPerf, Name: "降低渲染分辨率",
			Desc:       "按一半分辨率渲染再放大，同时加锐化，显卡吃力时帧数提升很大",
			SideEffect: "画面变糊",
			Lines:      []string{"r.ScreenPercentage 50", "r.Tonemapper.Sharpen 2"},
		},
		{
			ID: "statfps", Group: GroupPerf, Name: "显示帧数",
			Desc:  "在屏幕上显示帧数。这是开关命令，再执行一次就隐藏",
			Lines: []string{"stat fps"},
		},
		{
			ID: "skylight", Group: GroupCombat, Name: "提亮暗处",
			Desc:  "加强天光，关掉全局光照后室内、洞穴和夜里不再一片黑",
			Lines: []string{"r.SkylightIntensityMultiplier 3"},
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
		{ID: "perf", Name: "性能优化", Desc: "平衡优化 + 去阴影、关全局光照、关后处理，提亮暗处", Options: []string{"fps", "post", "shadows", "lumen", "debris", "skylight", "clouds", "fog", "underwater", "grass"}},
		{ID: "pvp", Name: "战斗", Desc: "性能优化 + 去天空、光柱、水面反光、粒子，低材质", Options: []string{"fps", "post", "shadows", "lumen", "debris", "textures", "skylight", "clouds", "fog", "sky", "lightshafts", "underwater", "water", "grass", "particles"}},
		// 畸变的洞穴靠全局光照上色，关掉后洞里会丢失颜色，所以不带 lumen
		{ID: "aberration", Name: "畸变", Desc: "战斗档去掉「关闭全局光照」，畸变洞里不丢颜色。这次游戏里已经关过全局光照的话，先重启游戏", Options: []string{"fps", "post", "shadows", "debris", "textures", "skylight", "clouds", "fog", "sky", "lightshafts", "underwater", "water", "grass", "particles"}},
	},
}
