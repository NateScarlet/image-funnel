package directory

type Repository interface {
	Save(dir *DirectoryInfo) error
	FindByPath(path string) (*DirectoryInfo, error)
	FindAll() ([]*DirectoryInfo, error)
}
