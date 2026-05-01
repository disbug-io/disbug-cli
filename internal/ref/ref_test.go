package ref

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSession(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want SessionRef
	}{
		{name: "normal session", arg: "7392", want: SessionRef{ID: 7392}},
		{name: "minimum positive session", arg: "1", want: SessionRef{ID: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSession(tt.arg)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	for _, arg := range []string{"", "abc", "-3", "7392.2", "7392 "} {
		t.Run("rejects "+arg, func(t *testing.T) {
			_, err := ParseSession(arg)

			require.Error(t, err)
		})
	}
}

func TestParsePin(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want PinRef
	}{
		{name: "normal pin", arg: "7392.2", want: PinRef{Session: 7392, Pin: 2}},
		{name: "minimum positive pin", arg: "1.1", want: PinRef{Session: 1, Pin: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePin(tt.arg)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	for _, arg := range []string{"", "7392", "7392.", ".2", "7392.2.3", "7392.x", "a.b"} {
		t.Run("rejects "+arg, func(t *testing.T) {
			_, err := ParsePin(arg)

			require.Error(t, err)
		})
	}
}

func TestParsePinFetch(t *testing.T) {
	t.Run("plain pin defaults to all", func(t *testing.T) {
		got, err := ParsePinFetch("7392.2", nil)

		require.NoError(t, err)
		require.Equal(t, PinFetch{Pin: PinRef{Session: 7392, Pin: 2}, Fields: []string{"all"}}, got)
	})

	t.Run("default fields apply without suffix", func(t *testing.T) {
		got, err := ParsePinFetch("7392.2", []string{"console", "screenshot"})

		require.NoError(t, err)
		require.Equal(t, PinFetch{Pin: PinRef{Session: 7392, Pin: 2}, Fields: []string{"screenshot", "console"}}, got)
	})

	t.Run("suffix overrides default fields", func(t *testing.T) {
		got, err := ParsePinFetch("7392.3:network,events", []string{"console"})

		require.NoError(t, err)
		require.Equal(t, PinFetch{Pin: PinRef{Session: 7392, Pin: 3}, Fields: []string{"network", "events"}}, got)
	})

	for _, arg := range []string{"7392.2:", "7392.2:all,console", "7392.2:unknown", "7392:console"} {
		t.Run("rejects "+arg, func(t *testing.T) {
			_, err := ParsePinFetch(arg, nil)

			require.Error(t, err)
		})
	}
}

func TestNormalizeFields(t *testing.T) {
	got, err := NormalizeFields([]string{"console", " console ", "screenshot"})

	require.NoError(t, err)
	require.Equal(t, []string{"screenshot", "console"}, got)
}

func TestNormalizeFieldsRejectsInvalidFields(t *testing.T) {
	tests := [][]string{
		{""},
		{" "},
		{"unknown"},
		{"all", "console"},
	}

	for _, fields := range tests {
		t.Run("rejects invalid fields", func(t *testing.T) {
			_, err := NormalizeFields(fields)

			require.Error(t, err)
		})
	}
}

func TestNormalizeFieldsAll(t *testing.T) {
	got, err := NormalizeFields([]string{"all"})

	require.NoError(t, err)
	require.Equal(t, []string{"all"}, got)
}

func TestIsKnownField(t *testing.T) {
	for _, field := range []string{"screenshot", "console", "network", "events", "replay", "voice_note", "video", "all"} {
		require.True(t, IsKnownField(field), field)
	}

	require.False(t, IsKnownField("console_logs"))
	require.False(t, IsKnownField(""))
}

func TestWireFieldName(t *testing.T) {
	require.Equal(t, "screenshot", WireFieldName("screenshot"))
	require.Equal(t, "console_logs", WireFieldName("console"))
	require.Equal(t, "network_logs", WireFieldName("network"))
	require.Equal(t, "user_events", WireFieldName("events"))
	require.Equal(t, "session_replay", WireFieldName("replay"))
	require.Equal(t, "voice_note", WireFieldName("voice_note"))
	require.Equal(t, "video_recording", WireFieldName("video"))
}

func TestDedupAndUnion(t *testing.T) {
	input := []PinFetch{
		{Pin: PinRef{Session: 7392, Pin: 2}, Fields: []string{"console"}},
		{Pin: PinRef{Session: 7392, Pin: 3}, Fields: []string{"events"}},
		{Pin: PinRef{Session: 7392, Pin: 2}, Fields: []string{"screenshot", "network"}},
		{Pin: PinRef{Session: 7392, Pin: 3}, Fields: []string{"all"}},
	}

	got := DedupAndUnion(input)

	require.Equal(t, []PinFetch{
		{Pin: PinRef{Session: 7392, Pin: 2}, Fields: []string{"screenshot", "console", "network"}},
		{Pin: PinRef{Session: 7392, Pin: 3}, Fields: []string{"all"}},
	}, got)
}
