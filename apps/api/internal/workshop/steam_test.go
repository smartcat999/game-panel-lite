package workshop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestParseCollectionID(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"123456789",
		"https://steamcommunity.com/sharedfiles/filedetails/?id=123456789",
		"https://www.steamcommunity.com/workshop/filedetails/?id=123456789",
	} {
		got, err := ParseCollectionID(input)
		if err != nil {
			t.Fatalf("ParseCollectionID(%q): %v", input, err)
		}
		if got != "123456789" {
			t.Fatalf("ParseCollectionID(%q) = %q", input, got)
		}
	}
}

func TestParseCollectionIDRejectsUntrustedURLs(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"http://steamcommunity.com/sharedfiles/filedetails/?id=123",
		"https://steamcommunity.com.evil.example/sharedfiles/filedetails/?id=123",
		"https://steamcommunity.com/profiles/123",
		"https://steamcommunity.com/sharedfiles/filedetails/?id=abc",
	} {
		if _, err := ParseCollectionID(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}

func TestSteamResolverExpandsCollectionAndFiltersAppID(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/ISteamRemoteStorage/GetCollectionDetails/v1/":
			id := r.Form.Get("publishedfileids[0]")
			if id == "100" {
				fmt.Fprint(w, `{"response":{"collectiondetails":[{"publishedfileid":"100","result":1,"children":[{"publishedfileid":"200","filetype":0},{"publishedfileid":"300","filetype":2}]}]}}`)
				return
			}
			if id == "300" {
				fmt.Fprint(w, `{"response":{"collectiondetails":[{"publishedfileid":"300","result":1,"children":[{"publishedfileid":"201","filetype":0}]}]}}`)
				return
			}
			t.Fatalf("unexpected collection id %q", id)
		case "/ISteamRemoteStorage/GetPublishedFileDetails/v1/":
			if r.Form.Get("itemcount") != "2" {
				t.Fatalf("unexpected itemcount %q", r.Form.Get("itemcount"))
			}
			fmt.Fprint(w, `{"response":{"publishedfiledetails":[
				{"publishedfileid":"200","result":1,"creator":"7656119","consumer_app_id":1281930,"file_size":"1024","preview_url":"https://steamusercontent.example/preview.png","title":"Calamity","description":"A mod","time_created":10,"time_updated":20,"file_type":0,"subscriptions":100,"favorited":4,"views":200,"tags":[{"tag":"New Content"}]},
				{"publishedfileid":"201","result":1,"creator":"7656120","consumer_app_id":322330,"file_size":"2048","title":"Wrong game","file_type":0}
			]}}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	resolver := NewSteamResolverWithClient(server.Client(), server.URL)
	collection, err := resolver.ResolveCollection(context.Background(), domain.ProviderTerrariaTModLoader, "100")
	if err != nil {
		t.Fatal(err)
	}
	if collection.ID != "100" || len(collection.Items) != 1 {
		t.Fatalf("unexpected collection: %+v", collection)
	}
	item := collection.Items[0]
	if item.WorkshopID != "200" || item.Title != "Calamity" || item.FileSize != 1024 {
		t.Fatalf("unexpected item: %+v", item)
	}
	if !strings.Contains(strings.Join(item.Tags, ","), "New Content") {
		t.Fatalf("expected Steam tags, got %+v", item.Tags)
	}
}

func TestSteamResolverRejectsUnavailableCollection(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"response":{"collectiondetails":[{"publishedfileid":"100","result":9}]}}`)
	}))
	defer server.Close()

	resolver := NewSteamResolverWithClient(server.Client(), server.URL)
	_, err := resolver.ResolveCollection(context.Background(), domain.ProviderTerrariaTModLoader, "100")
	if err == nil || !strings.Contains(err.Error(), "private, unavailable") {
		t.Fatalf("expected unavailable collection error, got %v", err)
	}
}
