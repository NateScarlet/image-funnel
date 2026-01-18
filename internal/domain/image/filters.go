package image

type ImageFilters struct {
	rating []int
}

func NewImageFilters(rating []int) *ImageFilters {
	return &ImageFilters{
		rating: rating,
	}
}

func (f *ImageFilters) Rating() []int {
	if f == nil {
		return nil
	}
	return f.rating
}
