package pq_types

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

// PostGISPoint is wrapper for PostGIS POINT type.
type PostGISPoint struct {
	Lon, Lat float64
}

// Value implements database/sql/driver Valuer interface.
// It returns point as WKT with SRID 4326 (WGS 84).
func (p PostGISPoint) Value() (driver.Value, error) {
	return []byte(fmt.Sprintf("SRID=4326;POINT(%.7f %.7f)", p.Lon, p.Lat)), nil
}

type ewkbPoint struct {
	ByteOrder byte   // 1 (LittleEndian)
	WkbType   uint32 // 0x20000001 (PointS)
	SRID      uint32 // 4326
	Point     PostGISPoint
}

// Scan implements database/sql Scanner interface.
// It expectes EWKB with SRID 4326 (WGS 84).
func (p *PostGISPoint) Scan(value interface{}) error {
	if value == nil {
		*p = PostGISPoint{}
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("pq_types: expected []byte, got %T (%v)", value, value)
	}

	ewkb := make([]byte, hex.DecodedLen(len(v)))
	n, err := hex.Decode(ewkb, v)
	if err != nil {
		return err
	}

	var ewkbP ewkbPoint
	err = binary.Read(bytes.NewReader(ewkb[:n]), binary.LittleEndian, &ewkbP)
	if err != nil {
		return err
	}

	if ewkbP.ByteOrder != 1 || ewkbP.WkbType != 0x20000001 || ewkbP.SRID != 4326 {
		return fmt.Errorf("pq_types: unexpected ewkb %#v", ewkbP)
	}
	*p = ewkbP.Point
	return nil
}

// check interfaces
var (
	_ driver.Valuer = PostGISPoint{}
	_ sql.Scanner   = &PostGISPoint{}
)

// Box2D type compatible with PostGIS Box2d type
type Box2D struct {
	Min, Max PostGISPoint
}

// Value implements database/sql/driver Valuer interface.
func (b Box2D) Value() (driver.Value, error) {
	return []byte(fmt.Sprintf("BOX(%.7f %.7f,%.7f %.7f)", b.Min.Lon, b.Min.Lat, b.Max.Lon, b.Max.Lat)), nil
}

// Scan implements database/sql Scanner interface.
func (b *Box2D) Scan(value interface{}) error {
	if value == nil {
		*b = Box2D{}
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("pq_types: expected []byte, got %T (%v)", value, value)
	}

	n, err := fmt.Sscanf(string(v), "BOX(%f %f,%f %f)", &b.Min.Lon, &b.Min.Lat, &b.Max.Lon, &b.Max.Lat)
	if err != nil {
		return err
	}
	if n != 4 {
		return fmt.Errorf("not enough params in the string: %v, %v != 4", v, n)
	}

	return nil
}

// check interfaces
var (
	_ driver.Valuer = Box2D{}
	_ sql.Scanner   = &Box2D{}
)

// Polygon type compatible with PostGIS POLYGON type
type Polygon struct {
	Points []PostGISPoint
}

// MakeEnvelope returns rectangular (min, max) polygon
func MakeEnvelope(min, max PostGISPoint) Polygon {
	return Polygon{
		Points: []PostGISPoint{min, {Lon: min.Lon, Lat: max.Lat}, max, {Lon: max.Lon, Lat: min.Lat}, min},
	}
}

// Min returns min side of rectangular polygon
func (p *Polygon) Min() PostGISPoint {
	if len(p.Points) != 5 || p.Points[0] != p.Points[4] ||
		p.Points[0].Lon != p.Points[1].Lon || p.Points[0].Lat != p.Points[3].Lat ||
		p.Points[1].Lat != p.Points[2].Lat || p.Points[2].Lon != p.Points[3].Lon {
		panic("Not an envelope polygon")
	}

	return p.Points[0]
}

// Max returns max side of rectangular polygon
func (p *Polygon) Max() PostGISPoint {
	if len(p.Points) != 5 || p.Points[0] != p.Points[4] ||
		p.Points[0].Lon != p.Points[1].Lon || p.Points[0].Lat != p.Points[3].Lat ||
		p.Points[1].Lat != p.Points[2].Lat || p.Points[2].Lon != p.Points[3].Lon {
		panic("Not an envelope polygon")
	}

	return p.Points[2]
}

// Value implements database/sql/driver Valuer interface.
func (p Polygon) Value() (driver.Value, error) {
	parts := make([]string, len(p.Points))
	for i, pt := range p.Points {
		parts[i] = fmt.Sprintf("%.7f %.7f", pt.Lon, pt.Lat)
	}
	return []byte(fmt.Sprintf("SRID=4326;POLYGON((%s))", strings.Join(parts, ","))), nil
}

type ewkbPolygon struct {
	ByteOrder byte   // 1 (LittleEndian)
	WkbType   uint32 // 0x20000003 (PolygonS)
	SRID      uint32 // 4326
	Rings     uint32
	Count     uint32
}

// Scan implements database/sql Scanner interface.
func (p *Polygon) Scan(value interface{}) error {
	if value == nil {
		*p = Polygon{}
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("pq_types: expected []byte, got %T (%v)", value, value)
	}

	ewkb := make([]byte, hex.DecodedLen(len(v)))
	_, err := hex.Decode(ewkb, v)
	if err != nil {
		return err
	}

	r := bytes.NewReader(ewkb)

	var ewkbP ewkbPolygon
	err = binary.Read(r, binary.LittleEndian, &ewkbP)
	if err != nil {
		return err
	}

	if ewkbP.ByteOrder != 1 || ewkbP.WkbType != 0x20000003 || ewkbP.SRID != 4326 || ewkbP.Rings != 1 {
		return fmt.Errorf("pq_types: unexpected ewkb %#v", ewkbP)
	}
	p.Points = make([]PostGISPoint, ewkbP.Count)

	err = binary.Read(r, binary.LittleEndian, p.Points)
	if err != nil {
		return err
	}

	return nil
}

// check interfaces
var (
	_ driver.Valuer = Polygon{}
	_ sql.Scanner   = &Polygon{}
)
