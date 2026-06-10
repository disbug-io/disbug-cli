package ref

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSession(t *testing.T) {
	reportURL := "https://staging.disbug.us/abb/projects/2/sessions/5/"
	tests := []struct {
		name string
		arg  string
		want SessionRef
	}{
		{name: "report url", arg: reportURL, want: SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}},
		{name: "report url with pin ignored", arg: reportURL + "?pin=1", want: SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSession(tt.arg)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	for _, arg := range []string{"", "abc", "-3", "0", "7392.2", "7392 ", "https://example.com/abb/sessions/5/"} {
		t.Run("rejects "+arg, func(t *testing.T) {
			_, err := ParseSession(arg)

			require.Error(t, err)
		})
	}
}

func TestParsePin(t *testing.T) {
	reportURL := "https://staging.disbug.us/abb/projects/2/sessions/5/"
	session := SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}
	tests := []struct {
		name string
		arg  string
		want PinRef
	}{
		{name: "pin query", arg: reportURL + "?pin=2", want: PinRef{Session: session, Pin: 2}},
		{name: "minimum positive pin query", arg: reportURL + "?pin=1", want: PinRef{Session: session, Pin: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePin(tt.arg)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	for _, arg := range []string{"", "7392", "7392.2", reportURL, reportURL + "?pin=0", reportURL + "?pin=x"} {
		t.Run("rejects "+arg, func(t *testing.T) {
			_, err := ParsePin(arg)

			require.Error(t, err)
		})
	}
}

func TestParsePinFetch(t *testing.T) {
	reportURL := "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2"
	pin := PinRef{Session: SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}, Pin: 2}

	t.Run("plain pin defaults to all", func(t *testing.T) {
		got, err := ParsePinFetch(reportURL, nil)

		require.NoError(t, err)
		require.Equal(t, PinFetch{Pin: pin, Fields: []string{"all"}}, got)
	})

	t.Run("default fields apply without suffix", func(t *testing.T) {
		got, err := ParsePinFetch(reportURL, []string{"console", "screenshot"})

		require.NoError(t, err)
		require.Equal(t, PinFetch{Pin: pin, Fields: []string{"screenshot", "console"}}, got)
	})

	t.Run("url fields override default fields", func(t *testing.T) {
		got, err := ParsePinFetch(reportURL+"&fields=network,events", []string{"console"})

		require.NoError(t, err)
		require.Equal(t, PinFetch{Pin: pin, Fields: []string{"network", "events"}}, got)
	})

	for _, arg := range []string{reportURL[:len(reportURL)-1], "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=0", "7392.2", "7392:console"} {
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
		{},
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
	session := SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}
	input := []PinFetch{
		{Pin: PinRef{Session: session, Pin: 2}, Fields: []string{"console"}},
		{Pin: PinRef{Session: session, Pin: 3}, Fields: []string{"events"}},
		{Pin: PinRef{Session: session, Pin: 2}, Fields: []string{"screenshot", "network"}},
		{Pin: PinRef{Session: session, Pin: 3}, Fields: []string{"all"}},
	}

	got := DedupAndUnion(input)

	require.Equal(t, []PinFetch{
		{Pin: PinRef{Session: session, Pin: 2}, Fields: []string{"screenshot", "console", "network"}},
		{Pin: PinRef{Session: session, Pin: 3}, Fields: []string{"all"}},
	}, got)
}

func TestDedupAndUnionCanonicalizesFirstOccurrence(t *testing.T) {
	session := SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}
	got := DedupAndUnion([]PinFetch{
		{Pin: PinRef{Session: session, Pin: 1}, Fields: []string{"network", "console"}},
	})

	require.Equal(t, []PinFetch{
		{Pin: PinRef{Session: session, Pin: 1}, Fields: []string{"console", "network"}},
	}, got)
}

func TestDedupAndUnionAllSupersedesSpecificFields(t *testing.T) {
	session := SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}
	got := DedupAndUnion([]PinFetch{
		{Pin: PinRef{Session: session, Pin: 1}, Fields: []string{"console"}},
		{Pin: PinRef{Session: session, Pin: 1}, Fields: []string{"all"}},
	})

	require.Equal(t, []PinFetch{
		{Pin: PinRef{Session: session, Pin: 1}, Fields: []string{"all"}},
	}, got)
}

func TestDedupAndUnionAllSupersedesSpecificFieldsInSameFetch(t *testing.T) {
	session := SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}
	got := DedupAndUnion([]PinFetch{
		{Pin: PinRef{Session: session, Pin: 1}, Fields: []string{"all", "console"}},
	})

	require.Equal(t, []PinFetch{
		{Pin: PinRef{Session: session, Pin: 1}, Fields: []string{"all"}},
	}, got)
}
