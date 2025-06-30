package document

// IntSet 整数集合，用于去重
type IntSet struct {
	items map[int]struct{}
}

// NewIntSet 创建新的整数集合
func NewIntSet() *IntSet {
	return &IntSet{
		items: make(map[int]struct{}),
	}
}

// Add 添加元素到集合
func (s *IntSet) Add(item int) {
	s.items[item] = struct{}{}
}

// Contains 检查元素是否在集合中
func (s *IntSet) Contains(item int) bool {
	_, exists := s.items[item]
	return exists
}

// Remove 从集合中移除元素
func (s *IntSet) Remove(item int) {
	delete(s.items, item)
}

// Size 返回集合大小
func (s *IntSet) Size() int {
	return len(s.items)
}

// Clear 清空集合
func (s *IntSet) Clear() {
	s.items = make(map[int]struct{})
}

// ToSlice 转换为切片
func (s *IntSet) ToSlice() []int {
	result := make([]int, 0, len(s.items))
	for item := range s.items {
		result = append(result, item)
	}
	return result
}

// Clone 克隆集合
func (s *IntSet) Clone() *IntSet {
	newSet := NewIntSet()
	for item := range s.items {
		newSet.Add(item)
	}
	return newSet
}
