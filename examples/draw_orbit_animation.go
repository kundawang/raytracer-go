package examples

import (
	"context"
	"fmt"
	"math"

	. "github.com/tiegz/raytracer-go/raytracer"
)

// RunCameraOrbitAnimation renders a "product showcase" animation: the camera
// orbits a model on a fixed circular track, one full revolution, at a
// constant angular speed. Keyframes are placed at the four compass points of
// the orbit; the last keyframe repeats the first pose so the loop is
// seamless. Frames are written to tmp/orbit_*.png.
func RunCameraOrbitAnimation(printProgress bool, jobs int) {
	world := NewWorld()
	world.Lights = []*AreaLight{
		NewPointLight(NewPoint(-10, 10, -10), NewColor(1, 1, 1)),
	}

	// the "product": a shiny sphere on a small pedestal
	product := NewSphere()
	product.SetTransform(NewTranslation(0, 1, 0))
	product.Material.Color = NewColor(0.8, 0.2, 0.2)
	product.Material.Reflective = 0.4

	pedestal := NewCylinder()
	pedestal.LocalShape.(*Cylinder).Minimum = 0
	pedestal.LocalShape.(*Cylinder).Maximum = 0.25
	pedestal.LocalShape.(*Cylinder).Closed = true
	pedestal.Material.Color = NewColor(0.9, 0.9, 0.9)

	floor := NewPlane()
	floor.Material.Pattern = NewCheckerPattern(Colors["Black"], Colors["White"])

	world.Objects = []*Shape{floor, pedestal, product}

	camera := NewCamera(320, 240, math.Pi/3)

	// one keyframe per quarter-revolution; the final keyframe repeats the
	// first pose so the animation loops without a visible seam
	const radius = 6.0
	const height = 2.5
	const secondsPerRevolution = 4.0
	target := NewPoint(0, 1, 0)
	up := NewVector(0, 1, 0)
	keyframes := []CameraKeyframe{}
	for i := 0; i <= 4; i++ {
		angle := float64(i) * (2 * math.Pi / 4)
		keyframes = append(keyframes, CameraKeyframe{
			Time: secondsPerRevolution * float64(i) / 4,
			From: NewPoint(radius*math.Sin(angle), height, radius*math.Cos(angle)),
			To:   target,
			Up:   up,
		})
	}

	animation, err := NewCameraAnimation(camera, 12, true, keyframes)
	if err != nil {
		fmt.Printf("Invalid animation: %s\n", err)
		return
	}

	fmt.Printf("Rendering %d frames (%.1fs at %.0f fps)...\n", animation.FrameCount(), animation.Duration(), animation.FPS)
	err = animation.Render(context.Background(), world, jobs, printProgress, func(frame int, canvas Canvas) error {
		filepath := fmt.Sprintf("tmp/orbit_%03d.png", frame)
		if err := canvas.SavePNG(filepath); err != nil {
			return err
		}
		fmt.Printf("Saved to %s\n", filepath)
		return nil
	})
	if err != nil {
		fmt.Printf("Animation failed: %s\n", err)
	}
}
