package domain

import (
	"errors"
	"sort"
)

// ErrAreaInvalida is returned when a preparation-area identifier cannot be
// used to build a comanda.
var ErrAreaInvalida = errors.New("El área de preparación es inválida")

// ComandaSourceItem is the minimum information needed to split a completed
// sale into one ticket per preparation area (RF-26). Names are snapshotted at
// sale time.
type ComandaSourceItem struct {
	VentaDetalleID int64
	ProductoID     int64
	Nombre         string
	Cantidad       int64
	AreaID         *int64
	AreaNombre     string
	AreaOrden      int
}

// ComandaGroup is the set of items that belong to one preparation area.
type ComandaGroup struct {
	AreaID     int64
	AreaNombre string
	Orden      int
	Items      []ComandaSourceItem
}

// SplitComandas groups sale items into exactly one ticket per preparation area
// (AC1). Items without an assigned area are returned in sinArea so the caller
// can warn the user; they never block completion (T-03 compatibility) and are
// not sent to any area.
//
// Ordering is deterministic: groups by area print order then area id, and items
// by sale-detail id. The function does not touch the database.
func SplitComandas(items []ComandaSourceItem) (groups []ComandaGroup, sinArea []ComandaSourceItem, err error) {
	byArea := make(map[int64]*ComandaGroup)
	areaOrder := make([]int64, 0)
	sinArea = make([]ComandaSourceItem, 0)

	for _, item := range items {
		if item.Cantidad <= 0 {
			return nil, nil, ErrCantidadInvalida
		}
		if item.AreaID == nil {
			sinArea = append(sinArea, item)
			continue
		}
		areaID := *item.AreaID
		if areaID <= 0 {
			return nil, nil, ErrAreaInvalida
		}
		group, ok := byArea[areaID]
		if !ok {
			group = &ComandaGroup{
				AreaID:     areaID,
				AreaNombre: item.AreaNombre,
				Orden:      item.AreaOrden,
			}
			byArea[areaID] = group
			areaOrder = append(areaOrder, areaID)
		}
		group.Items = append(group.Items, item)
	}

	groups = make([]ComandaGroup, 0, len(areaOrder))
	for _, areaID := range areaOrder {
		group := byArea[areaID]
		sort.SliceStable(group.Items, func(i, j int) bool {
			return group.Items[i].VentaDetalleID < group.Items[j].VentaDetalleID
		})
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Orden != groups[j].Orden {
			return groups[i].Orden < groups[j].Orden
		}
		return groups[i].AreaID < groups[j].AreaID
	})
	sort.SliceStable(sinArea, func(i, j int) bool {
		return sinArea[i].VentaDetalleID < sinArea[j].VentaDetalleID
	})
	return groups, sinArea, nil
}
