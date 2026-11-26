// Package meter is the infrastructure producer of the Modbus driver. It maps
// the meters, the grid connection and the site and consumption values the
// driver reaches to the resources, relations and facts of the infrastructure
// pack, in SI units. It reads native evaluations that other packages decode
// and sends no request. Every native value stays readable in the driver
// namespace beside the facts, whether or not an element of the pack takes it.
package meter
