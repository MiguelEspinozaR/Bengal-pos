package domain

import (
	"errors"
	"testing"
)

func areaID(id int64) *int64 { return &id }

func TestSplitComandasByArea(t *testing.T) {
	items := []ComandaSourceItem{
		{VentaDetalleID: 1, ProductoID: 10, Nombre: "Hamburguesa", Cantidad: 2, AreaID: areaID(1), AreaNombre: "Cocina", AreaOrden: 1},
		{VentaDetalleID: 2, ProductoID: 20, Nombre: "Cerveza", Cantidad: 1, AreaID: areaID(2), AreaNombre: "Barra", AreaOrden: 2},
		{VentaDetalleID: 3, ProductoID: 11, Nombre: "Papas fritas", Cantidad: 1, AreaID: areaID(1), AreaNombre: "Cocina", AreaOrden: 1},
	}

	groups, sinArea, err := SplitComandas(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if len(sinArea) != 0 {
		t.Fatalf("sinArea = %d, want 0", len(sinArea))
	}

	if groups[0].AreaID != 1 || groups[0].AreaNombre != "Cocina" {
		t.Fatalf("first group = %+v", groups[0])
	}
	if len(groups[0].Items) != 2 || groups[0].Items[0].Nombre != "Hamburguesa" || groups[0].Items[1].Nombre != "Papas fritas" {
		t.Fatalf("kitchen items = %+v", groups[0].Items)
	}
	if groups[1].AreaID != 2 || len(groups[1].Items) != 1 || groups[1].Items[0].Nombre != "Cerveza" {
		t.Fatalf("bar group = %+v", groups[1])
	}
}

func TestSplitComandasKeepsSnapshotNames(t *testing.T) {
	items := []ComandaSourceItem{
		{VentaDetalleID: 7, ProductoID: 10, Nombre: "Nombre vendido", Cantidad: 3, AreaID: areaID(9), AreaNombre: "Postres", AreaOrden: 4},
	}
	groups, _, err := SplitComandas(items)
	if err != nil {
		t.Fatal(err)
	}
	if got := groups[0].Items[0].Nombre; got != "Nombre vendido" {
		t.Fatalf("snapshot name = %q", got)
	}
	if got := groups[0].Items[0].Cantidad; got != 3 {
		t.Fatalf("quantity = %d", got)
	}
}

func TestSplitComandasWithoutAreaGoesToSinArea(t *testing.T) {
	items := []ComandaSourceItem{
		{VentaDetalleID: 2, ProductoID: 20, Nombre: "Sin área", Cantidad: 1},
		{VentaDetalleID: 1, ProductoID: 10, Nombre: "Cocina", Cantidad: 1, AreaID: areaID(1), AreaNombre: "Cocina", AreaOrden: 1},
	}
	groups, sinArea, err := SplitComandas(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].AreaID != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	if len(sinArea) != 1 || sinArea[0].Nombre != "Sin área" {
		t.Fatalf("sinArea = %+v", sinArea)
	}
}

func TestSplitComandasDeterministicOrder(t *testing.T) {
	items := []ComandaSourceItem{
		{VentaDetalleID: 30, ProductoID: 30, Nombre: "Barra 3", Cantidad: 1, AreaID: areaID(2), AreaNombre: "Barra", AreaOrden: 2},
		{VentaDetalleID: 20, ProductoID: 20, Nombre: "Cocina 2", Cantidad: 1, AreaID: areaID(1), AreaNombre: "Cocina", AreaOrden: 1},
		{VentaDetalleID: 10, ProductoID: 10, Nombre: "Cocina 1", Cantidad: 1, AreaID: areaID(1), AreaNombre: "Cocina", AreaOrden: 1},
		{VentaDetalleID: 40, ProductoID: 40, Nombre: "Barra 4", Cantidad: 1, AreaID: areaID(2), AreaNombre: "Barra", AreaOrden: 2},
	}
	groups, _, err := SplitComandas(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].AreaID != 1 || groups[1].AreaID != 2 {
		t.Fatalf("group order = %+v", groups)
	}
	if groups[0].Items[0].VentaDetalleID != 10 || groups[0].Items[1].VentaDetalleID != 20 {
		t.Fatalf("kitchen item order = %+v", groups[0].Items)
	}
	if groups[1].Items[0].VentaDetalleID != 30 || groups[1].Items[1].VentaDetalleID != 40 {
		t.Fatalf("bar item order = %+v", groups[1].Items)
	}
}

func TestSplitComandasRejectsInvalidQuantity(t *testing.T) {
	_, _, err := SplitComandas([]ComandaSourceItem{
		{VentaDetalleID: 1, ProductoID: 1, Cantidad: 0, AreaID: areaID(1)},
	})
	if !errors.Is(err, ErrCantidadInvalida) {
		t.Fatalf("err = %v, want ErrCantidadInvalida", err)
	}
}
