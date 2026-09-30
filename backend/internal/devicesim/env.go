package devicesim

import (
	"math"
	"math/rand"
	"time"
)

// Environment produces plausible sensor readings: a daily temperature and light cycle, wetter and darker while
// raining, plus a little noise. It is deterministic for a given seed.
type Environment struct {
	rng *rand.Rand
}

func NewEnvironment(seed int64) *Environment {
	return &Environment{rng: rand.New(rand.NewSource(seed))}
}

// Sample returns readings for the local time of now. raining should be the device's current verdict.
func (e *Environment) Sample(now time.Time, raining bool) Sensors {
	hour := float64(now.Hour()) + float64(now.Minute())/60
	daylight := math.Max(0, math.Sin(math.Pi*(hour-6)/12)) // 0 at 06:00 and 18:00, peak at noon

	temp := 26 + 5*math.Sin(2*math.Pi*(hour-9)/24) + e.rng.NormFloat64()*0.15
	hum := 78 - 18*daylight + e.rng.NormFloat64()*0.6
	light := 150 + 3700*daylight + e.rng.NormFloat64()*25
	if raining {
		temp -= 3
		hum += 18
		light *= 0.3
	}
	return Sensors{
		Temp:     math.Round(temp*10) / 10,
		Humidity: math.Round(math.Min(99, math.Max(30, hum))),
		Light:    int(math.Min(4095, math.Max(0, light))),
	}
}
