package progress

import (
	// "fmt"
	"os"
	"time"

	// "github.com/jedib0t/go-pretty/v6/progress" //不再直接使用go-pretty的Writer和Tracker
	"github.com/jedib0t/go-pretty/v6/text" // 仅用于颜色定义
	"go.uber.org/zap"
)

// Adapter теперь просто обертка вокруг нашего кастомного Tracker,
// чтобы соответствовать интерфейсу, который ожидает NewProgressTracker.
// Или, если NewProgressTracker может напрямую использовать *Tracker, то Adapter может быть удален.
// Пока что оставим Adapter как простую проксю.
type Adapter struct {
	tracker *Tracker // Только наш кастомный трекер
	logger  *zap.Logger
}

// NewAdapter создает новый адаптер进度.
func NewAdapter(logger *zap.Logger) *Adapter {
	// Создаем и настраиваем наш кастомный Tracker
	tracker := NewTracker(
		0, // totalUnits будет установлен позже через SetTotal или при первом Update
		WithUnit("字符", "chars"),
		WithMessage("翻译进度"), // Это сообщение будет отображаться Tracker-ом
		WithBarStyle(50, "█", "░", "[", "]"),
		WithCost(0.00002, "$"), // Примерная стоимость, если нужна
		WithColors(
			text.Colors{text.FgHiWhite, text.Bold},
			text.Colors{text.FgCyan},
			text.Colors{text.FgHiBlack},
			text.Colors{text.FgGreen},
			text.Colors{text.FgYellow},
			text.Colors{text.FgMagenta},
			text.Colors{text.FgWhite},
		),
		WithVisibility(
			true, // showPercent
			true, // showBar
			true, // showStats
			true, // showTime
			true, // showETA
			true, // showCost
			true, // showSpeed
		),
		WithWriter(os.Stderr),
		WithRefreshInterval(time.Second),
	)

	return &Adapter{
		tracker: tracker,
		logger:  logger,
	}
}

// Start начинает отслеживание прогресса.
func (pa *Adapter) Start() {
	pa.tracker.Start()
}

// Update обновляет прогресс.
func (pa *Adapter) Update(completedUnits int64) {
	// Обновляем наш кастомный трекер
	// Если totalUnits в трекере еще не установлен или меньше текущего completedUnits (что странно, но на всякий случай)
	// Это должно управляться через SetTotal извне, но можно добавить защитный механизм.
	if pa.tracker.totalUnits == 0 && completedUnits > 0 {
		// Попытка установить totalUnits, если он еще не был установлен.
		// Однако, правильнее устанавливать totalUnits заранее через SetTotal.
		// Здесь мы не знаем реальный total, поэтому это может быть неточно.
		// pa.tracker.SetTotal(completedUnits * 2) // Пример: предполагаем, что это половина работы
	} else if completedUnits > pa.tracker.totalUnits && pa.tracker.totalUnits > 0 {
		// Если завершенных юнитов стало больше общего количества, обновляем общее количество
		// pa.tracker.SetTotal(completedUnits)
	}
	pa.tracker.Update(completedUnits)

	// Логирование через zap, если необходимо
	if pa.logger != nil {
		// pa.logger.Debug("Progress adapter updated", zap.Int64("completed", completedUnits))
	}
}

// Stop останавливает отслеживание прогресса.
func (pa *Adapter) Stop() {
	pa.tracker.Stop()
}

// Done помечает прогресс как выполненный.
func (pa *Adapter) Done(summary *SummaryStats) {
	// Убедимся, что трекер показывает 100% перед завершением
	// и что totalUnits был установлен.
	if pa.tracker != nil { // Убедимся, что трекер существует
		if pa.tracker.totalUnits > 0 && pa.tracker.completedUnits < pa.tracker.totalUnits {
			pa.tracker.Update(pa.tracker.totalUnits) // Обновить до 100%
		}
		pa.tracker.Done(summary) // Передать summary дальше
	} else {
		// Если трекер nil, возможно, стоит залогировать ошибку или предупреждение
		if pa.logger != nil {
			pa.logger.Warn("Adapter.Done called but internal tracker is nil")
		}
	}
}

// GetTracker возвращает кастомный трекер.
func (pa *Adapter) GetTracker() *Tracker {
	return pa.tracker
}

// GetWriter больше не нужен, так как мы не управляем go-pretty.Writer напрямую здесь.
// Можно удалить или вернуть nil, если интерфейс требует его наличия.
/*
func (pa *Adapter) GetWriter() progress.Writer {
	return nil // Или удалить метод
}
*/
