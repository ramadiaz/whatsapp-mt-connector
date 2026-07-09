package service

import (
	"strings"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
)

var synonymMap = map[string][]string{
	"food":      {"kopi", "makan", "restoran", "minum", "cafe", "coffee", "lunch", "dinner", "breakfast", "bakery", "warung", "nasi", "ayam", "seafood", "pizza", "burger", "snack", "camilan"},
	"transport": {"bensin", "parkir", "tol", "grab", "gojek", "taxi", "busway", "kereta", "ojek", "mrt", "bus", "angkot", "motor", "bbm", "bbm"},
	"shopping":  {"marketplace", "baju", "sepatu", "shopee", "tokopedia", "lazada", "belanja", "toko", "pakaian", "fashion", "beli"},
	"income":    {"gaji", "fee", "bonus", "freelance", "salary", "honor", "pendapatan", "revenue", "transfer masuk", "penghasilan"},
	"health":    {"obat", "dokter", "apotik", "apotek", "klinik", "rumah sakit", "vitamin", "rs", "puskesmas"},
	"education": {"buku", "kursus", "sekolah", "kampus", "kuliah", "les", "workshop", "seminar", "training"},
	"utility":   {"listrik", "pln", "air", "internet", "wifi", "telkom", "indihome", "pdam", "gas"},
	"entertainment": {"bioskop", "netflix", "spotify", "game", "hiburan", "cinema", "konser", "streaming"},
}

func MatchCategory(hint string, categories []transaction.Category) *transaction.Category {
	if hint == "" {
		return nil
	}
	hintLower := strings.ToLower(strings.TrimSpace(hint))

	for i := range categories {
		catTitleLower := strings.ToLower(categories[i].Title)
		if catTitleLower == hintLower {
			return &categories[i]
		}
	}

	for i := range categories {
		catTitleLower := strings.ToLower(categories[i].Title)
		if strings.Contains(catTitleLower, hintLower) || strings.Contains(hintLower, catTitleLower) {
			return &categories[i]
		}
	}

	for canonicalKey, synonyms := range synonymMap {
		for _, syn := range synonyms {
			if strings.Contains(hintLower, syn) || hintLower == syn {
				for i := range categories {
					if strings.Contains(strings.ToLower(categories[i].Title), canonicalKey) ||
						strings.Contains(strings.ToLower(categories[i].Title), "category_"+canonicalKey) {
						return &categories[i]
					}
				}
				break
			}
		}
	}

	return nil
}

func MatchAccount(hint string, accounts []transaction.Account) *transaction.Account {
	if hint == "" {
		return nil
	}
	hintLower := strings.ToLower(strings.TrimSpace(hint))
	for i := range accounts {
		if strings.Contains(strings.ToLower(accounts[i].Name), hintLower) {
			return &accounts[i]
		}
	}
	return nil
}

func CategoryLabels(categories []transaction.Category) []string {
	labels := make([]string, len(categories))
	for i, c := range categories {
		labels[i] = c.Title
	}
	return labels
}

func AccountLabels(accounts []transaction.Account) []string {
	labels := make([]string, len(accounts))
	for i, a := range accounts {
		labels[i] = a.Name
	}
	return labels
}
