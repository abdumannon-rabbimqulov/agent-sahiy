package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Delivery (O'zbekistondagi yetkazma) API — support'dan alohida server va
// alohida login. Token service-token.json da saqlanadi.
const (
	ServiceBaseURL   = "https://api.sahiy.uz"
	ServiceLoginPath = "/api/v2/service/user/login"
	DeliveryPath     = "/api/v2/admin/delivery/orders/filter"
	ServiceTokenFile = "service-token.json"
	ServiceTTL       = 24 * time.Hour // expires_in kelmasa
)

// Service — delivery API'ga kirish ma'lumotlari (.env dagi SERVICE_*).
// Device maydonlari majburiy: ularsiz server 500 qaytaradi.
type Service struct {
	BaseURL    string
	Phone      string
	Password   string
	APKType    int
	DeviceID   string
	DeviceName string
	DeviceType string
	FcmToken   string
}

// ServiceFromEnv .env dagi SERVICE_* qiymatlarini o'qiydi.
func ServiceFromEnv() Service {
	apk, _ := strconv.Atoi(os.Getenv("SERVICE_APK_TYPE"))
	s := Service{
		BaseURL:    os.Getenv("SERVICE_BASE_URL"),
		Phone:      os.Getenv("SERVICE_PHONE"),
		Password:   os.Getenv("SERVICE_PASSWORD"),
		APKType:    apk,
		DeviceID:   os.Getenv("SERVICE_DEVICE_ID"),
		DeviceName: os.Getenv("SERVICE_DEVICE_NAME"),
		DeviceType: os.Getenv("SERVICE_DEVICE_TYPE"),
		FcmToken:   os.Getenv("SERVICE_FCM_TOKEN"),
	}
	if s.BaseURL == "" {
		s.BaseURL = ServiceBaseURL
	}
	return s
}

// ServiceLogin yangi token oladi (cache'ga tegmaydi).
func ServiceLogin(s Service) (string, time.Duration, error) {
	if s.Phone == "" || s.Password == "" {
		return "", 0, fmt.Errorf("SERVICE_PHONE yoki SERVICE_PASSWORD berilmagan")
	}
	body, err := json.Marshal(map[string]any{
		"phone":       s.Phone,
		"password":    s.Password,
		"apk_type":    s.APKType,
		"device_id":   s.DeviceID,
		"device_name": s.DeviceName,
		"device_type": s.DeviceType,
		"fcm_token":   s.FcmToken,
	})
	if err != nil {
		return "", 0, fmt.Errorf("body marshal: %w", err)
	}

	base := s.BaseURL
	if base == "" {
		base = ServiceBaseURL
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+ServiceLoginPath, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("so'rov yaratish: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("so'rov yuborish: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("service login (status %d): %s", resp.StatusCode, snippet(raw))
	}

	// expires_in ba'zan son, ba'zan matn ("2592000") bo'lib keladi.
	var lr struct {
		AccessToken string      `json:"access_token"`
		ExpiresIn   json.Number `json:"expires_in"`
		Message     string      `json:"message"`
		Error       string      `json:"error"`
	}
	if err := json.Unmarshal(raw, &lr); err != nil {
		return "", 0, fmt.Errorf("login javobini o'qish: %w", err)
	}
	if lr.AccessToken == "" {
		msg := lr.Error
		if msg == "" {
			msg = lr.Message
		}
		return "", 0, fmt.Errorf("service login muvaffaqiyatsiz: %s", msg)
	}

	ttl := ServiceTTL
	if sec, err := lr.ExpiresIn.Int64(); err == nil && sec > 0 {
		ttl = time.Duration(sec) * time.Second
	}
	// 5 daqiqa zaxira — muddat tugashiga yaqin qayta login qilinsin.
	if ttl > 5*time.Minute {
		ttl -= 5 * time.Minute
	}
	return lr.AccessToken, ttl, nil
}

// ServiceToken avval cache'ni tekshiradi, bo'lmasa login qilib saqlaydi.
func ServiceToken(s Service, cachePath string) (string, error) {
	if tc := LoadToken(cachePath); tc != nil {
		return tc.Token, nil
	}
	return serviceRefresh(s, cachePath)
}

// ServiceRefresh cache'ni o'chirib yangi token oladi (401 kelganda).
func ServiceRefresh(s Service, cachePath string) (string, error) {
	DropToken(cachePath)
	return serviceRefresh(s, cachePath)
}

func serviceRefresh(s Service, cachePath string) (string, error) {
	token, ttl, err := ServiceLogin(s)
	if err != nil {
		return "", err
	}
	if err := SaveToken(cachePath, token, ttl); err != nil {
		fmt.Printf("ogohlantirish: service token cache'ga saqlanmadi: %v\n", err)
	}
	return token, nil
}

// DeliveryFilter — qidiruv sharti: track raqami yoki user_id (= support'dagi
// client_id).
type DeliveryFilter struct {
	TrackNumber string `json:"track_number"`
	UserID      int64  `json:"user_id"`
	Page        int    `json:"page"`
	Size        int    `json:"size"`
}

// DeliveryOrder — yetkazma buyurtmasidan olinadigan maydonlar.
type DeliveryOrder struct {
	FullName       string `json:"full_name"`
	Phone          string `json:"phone"`
	Address        string `json:"address"`
	LocationNumber string `json:"location_number"`
	ExpressNum     string `json:"express_num"`
	BranchName     string `json:"branch_name"`
	CreatedAt      string `json:"created_at"`
	UserID         int64  `json:"user_id"`

	// Delivered — mijoz buyurtmani olib ketganmi.
	Delivered bool `json:"delivered"`
	// DeliveredAt — qachon olib ketilgani (delivered=true bo'lsa).
	DeliveredAt string `json:"delivered_at,omitempty"`
	// City, BranchAddress — qaysi filialda ekanini aytish uchun.
	City          string `json:"city,omitempty"`
	BranchAddress string `json:"branch_address,omitempty"`
}

// FetchDelivery yetkazma buyurtmalarini oladi.
// `delivered` majburiy filtr: usiz server doim NO ORDERS FOUND qaytaradi —
// shuning uchun false va true alohida so'raladi va natijalar birlashtiriladi.
func FetchDelivery(s Service, token string, f DeliveryFilter) ([]DeliveryOrder, error) {
	if f.TrackNumber == "" && f.UserID <= 0 {
		return nil, fmt.Errorf("track_number yoki user_id berilmagan")
	}
	var all []DeliveryOrder
	for _, delivered := range []string{"false", "true"} {
		part, err := fetchDeliveryPage(s, token, f, delivered)
		if err != nil {
			return nil, err
		}
		all = append(all, part...)
	}
	return all, nil
}

func fetchDeliveryPage(s Service, token string, f DeliveryFilter, delivered string) ([]DeliveryOrder, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 {
		f.Size = 20
	}
	base := s.BaseURL
	if base == "" {
		base = ServiceBaseURL
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(f.Page))
	q.Set("size", strconv.Itoa(f.Size))
	q.Set("delivered", delivered)
	if f.TrackNumber != "" {
		q.Set("track_number", f.TrackNumber)
	} else {
		q.Set("user_id", strconv.FormatInt(f.UserID, 10))
	}

	deliveryURL := strings.TrimRight(base, "/") + DeliveryPath + "?" + q.Encode()
	newReq := func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, deliveryURL, nil)
		if err != nil {
			return nil, fmt.Errorf("so'rov yaratish: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	status, raw, err := doWithRetry(client, newReq, Retries())
	if err != nil {
		return nil, fmt.Errorf("so'rov yuborish: %w", err)
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, ErrUnauthorized
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("yetkazma buyurtmalari (status %d): %s", status, snippet(raw))
	}

	// Buyurtma topilmasa `data` obyekt emas, bo'sh massiv `[]` bo'lib keladi.
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("javobni o'qish: %w", err)
	}
	var data struct {
		Orders []map[string]any `json:"orders"`
	}
	if err := json.Unmarshal(out.Data, &data); err != nil {
		return nil, nil // massiv keldi — buyurtma yo'q
	}

	orders := make([]DeliveryOrder, 0, len(data.Orders))
	for _, m := range data.Orders {
		orders = append(orders, DeliveryOrder{
			FullName:       str(get(m, "full_name")),
			Phone:          str(get(m, "phone")),
			Address:        str(get(m, "address")),
			LocationNumber: str(get(m, "location_number")),
			ExpressNum:     str(get(m, "express_num")),
			// branch_name ba'zan yuqorida, ba'zan delivery_address ichida.
			BranchName: str(first(m, "branch_name", "delivery_address.branch_name",
				"station.name", "delivery_address.branch_name_uz")),
			CreatedAt: str(get(m, "created_at")),
			UserID:    num64(get(m, "user_id")),

			Delivered:   truthy(get(m, "delivered")),
			DeliveredAt: str(get(m, "delivered_at")),
			City:        str(get(m, "city")),
			BranchAddress: str(first(m,
				"delivery_address.branch_address",
				"station.address",
				"address_info.address",
			)),
		})
	}
	return orders, nil
}

// truthy — bool, son yoki matn ko'rinishidagi "ha" ni tushunadi.
func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x == "true" || x == "1"
	}
	return false
}

// DeliveryJSON buyurtmalarni tayyor JSON matn qilib qaytaradi.
func DeliveryJSON(s Service, token string, f DeliveryFilter) ([]byte, error) {
	orders, err := FetchDelivery(s, token, f)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(struct {
		Count  int             `json:"count"`
		Orders []DeliveryOrder `json:"orders"`
	}{len(orders), orders}, "", "  ")
}
