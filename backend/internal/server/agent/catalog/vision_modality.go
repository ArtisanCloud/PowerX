package catalog

// normalizeVisionModality 按明确的 operation 分离历史 image 下的视觉声明。
// 不按型号名称推断，也不修改源 YAML 或租户配置。
func normalizeVisionModality(manifest *Manifest) {
	move := func(mods map[string]ModalityManifest) {
		image, vision := mods["image"], mods["vlm"]
		remaining := []ModelManifest{}
		for _, model := range image.Models {
			op, ok := model.Operations[VisionUnderstanding]
			if !ok {
				remaining = append(remaining, model)
				continue
			}
			exists := false
			for _, current := range vision.Models {
				if current.ID == model.ID {
					exists = true
					break
				}
			}
			if !exists {
				copy := model
				copy.Operations = map[string]MediaOperation{VisionUnderstanding: op}
				vision.Models = append(vision.Models, copy)
			}
			other := map[string]MediaOperation{}
			for key, operation := range model.Operations {
				if key != VisionUnderstanding {
					other[key] = operation
				}
			}
			if len(other) > 0 {
				copy := model
				copy.Operations = other
				remaining = append(remaining, copy)
			}
			if manifest.Drivers == nil {
				manifest.Drivers = map[string]string{}
			}
			if manifest.Drivers["vlm"] == "" {
				manifest.Drivers["vlm"] = op.Driver
			}
		}
		if len(vision.Models) > 0 {
			mods["vlm"] = vision
		}
		if _, ok := mods["image"]; ok {
			mods["image"] = ModalityManifest{Models: remaining}
		}
	}
	move(manifest.Modalities)
	for _, app := range manifest.Apps {
		move(app.Modalities)
	}
}
