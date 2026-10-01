package devicesim

import (
	"math"
	"math/rand"
	"time"
)

// Environment produces plausible sensor readings: a daily temperature and light cycle, wetter and darker while
// raining, a rain plate that is bone dry or soaked, plus a little noise. It is deterministic for a given seed.
type Environment struct {
	rng *rand.Rand
}

// Conditions is what the outside world is doing, as seen by the sensors.
type Conditions struct {
	Rainy    bool // the air looks rainy: wetter, darker, cooler
	PlateWet bool // water is actually on the rain plate
}

func NewEnvironment(seed int64) *Environment {
	return &Environment{rng: rand.New(rand.NewSource(seed))}
}

// Sample returns readings for the local time of now. The rain plate reads about 20 when dry and about 2600 when
// wet, far from both thresholds, so the noise never flips a verdict by itself.
func (e *Environment) Sample(now time.Time, c Conditions) Sensors {
	hour := float64(now.Hour()) + float64(now.Minute())/60
	daylight := math.Max(0, math.Sin(math.Pi*(hour-6)/12)) // 0 at 06:00 and 18:00, peak at noon

	temp := 26 + 5*math.Sin(2*math.Pi*(hour-9)/24) + e.rng.NormFloat64()*0.15
	hum := 78 - 18*daylight + e.rng.NormFloat64()*0.6
	light := 150 + 3700*daylight + e.rng.NormFloat64()*25
	if c.Rainy {
		temp -= 3
		hum += 18
		light *= 0.3
	}
	plate := 20 + math.Abs(e.rng.NormFloat64())*6
	if c.PlateWet {
		plate = 2600 + e.rng.NormFloat64()*150
	}
	return Sensors{
		Temp:      math.Round(temp*10) / 10,
		Humidity:  math.Round(math.Min(99, math.Max(30, hum))),
		Light:     int(math.Min(4095, math.Max(0, light))),
		RainValid: true,
		RainLevel: int(math.Min(4095, math.Max(0, plate))),
	}
}
