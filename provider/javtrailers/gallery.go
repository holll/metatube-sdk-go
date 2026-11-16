package javtrailers

import "encoding/json"

// nuxtGallery reads Nuxt's devalue reference table without evaluating scripts.
// Missing/changed/invalid optional gallery data must not break movie metadata.
func nuxtGallery(raw []byte, contentID string) []string {
	var nodes []json.RawMessage
	if json.Unmarshal(raw, &nodes) != nil || len(nodes) == 0 {
		return nil
	}
	valid := func(i int) bool { return i >= 0 && i < len(nodes) }
	object := func(i int) map[string]int {
		// Nuxt can wrap objects in Reactive/Ref tags. Limit traversal to reject
		// cyclic or malformed reference graphs instead of recursing forever.
		for steps := 0; valid(i) && steps < len(nodes); steps++ {
			var obj map[string]int
			if json.Unmarshal(nodes[i], &obj) == nil && obj != nil {
				return obj
			}
			var wrapper []json.RawMessage
			var tag string
			var next int
			if json.Unmarshal(nodes[i], &wrapper) != nil || len(wrapper) != 2 || json.Unmarshal(wrapper[0], &tag) != nil || json.Unmarshal(wrapper[1], &next) != nil {
				return nil
			}
			switch tag {
			case "Reactive", "ShallowReactive", "Ref", "ShallowRef":
				i = next
			default:
				return nil
			}
		}
		return nil
	}
	lookup := func(i int, key string) (int, bool) {
		v, ok := object(i)[key]
		return v, ok && valid(v)
	}
	index := 0
	for _, key := range []string{"data", "video-" + contentID, "video"} {
		var ok bool
		if index, ok = lookup(index, key); !ok {
			return nil
		}
	}
	identity, ok := lookup(index, "contentId")
	var actualID string
	if !ok || json.Unmarshal(nodes[identity], &actualID) != nil || actualID != contentID {
		return nil
	}
	gallery, ok := lookup(index, "gallery")
	if !ok {
		return nil
	}
	var refs []int
	if json.Unmarshal(nodes[gallery], &refs) != nil {
		return nil
	}
	images := []string{}
	for _, ref := range refs {
		var image string
		if valid(ref) && json.Unmarshal(nodes[ref], &image) == nil {
			images = append(images, image)
		}
	}
	return images
}
