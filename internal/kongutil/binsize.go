package kongutil

import (
	"fmt"
	"math"
	"reflect"

	"github.com/docker/go-units"

	"github.com/alecthomas/kong"
)

var BinSizeMapper = kong.NamedMapper("binsize", kong.MapperFunc(binSizeMapper))

func binSizeMapper(dctx *kong.DecodeContext, target reflect.Value) error {
	var maxValue uint64

	switch target.Kind() {
	case reflect.Int:
		maxValue = math.MaxInt64
	case reflect.Int8:
		maxValue = math.MaxInt8
	case reflect.Int16:
		maxValue = math.MaxInt16
	case reflect.Int32:
		maxValue = math.MaxInt32
	case reflect.Int64:
		maxValue = math.MaxInt64
	case reflect.Uint, reflect.Uintptr:
		maxValue = math.MaxUint
	case reflect.Uint8:
		maxValue = math.MaxUint8
	case reflect.Uint16:
		maxValue = math.MaxUint16
	case reflect.Uint32:
		maxValue = math.MaxUint32
	case reflect.Uint64:
		maxValue = math.MaxUint64
	default:
		return fmt.Errorf("\"binsize\" can only be used with integer types")
	}

	var rawSize string
	err := dctx.Scan.PopValueInto("memsize", &rawSize)
	if err != nil {
		return err
	}

	memSize, err := units.RAMInBytes(rawSize)
	if err != nil {
		return err
	}

	if memSize < 0 || uint64(memSize) > maxValue {
		return fmt.Errorf("value out of range")
	}

	target.Set(reflect.ValueOf(memSize).Convert(target.Type()))

	return nil
}
