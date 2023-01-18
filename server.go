package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
)

type Method string
type ContentType string

const (
	LIST    Method = "LIST" // NewRoute duplicates into GET/POST
	POST    Method = "POST"
	GET     Method = "GET"
	PUT     Method = "PUT"
	DELETE  Method = "DELETE"
	PATCH   Method = "PATCH"
	OPTIONS Method = "OPTIONS"
)

const (
	EMPTY ContentType = ""
	JSON  ContentType = "application/json; charset=utf-8"
	TEXT  ContentType = "text/plain; charset=utf-8"
)

const (
	ROLE     = "Role"
	USER     = "Username"
	PASSWORD = "Password"
)

type ctxKey struct{}

type Route struct {
	method  Method
	regex   *regexp.Regexp
	handler http.HandlerFunc
}

type serverConfig struct {
	allowedHosts []string
	secretKey    []byte
}

var server serverConfig

func NewServer(routes []Route) error {
	accessFile, err := os.Open("./config.json")
	if err != nil {
		return err
	}
	defer accessFile.Close()

	jsonFile, err := ioutil.ReadAll(accessFile)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(jsonFile), &result)

	server.secretKey = []byte(result["server_secret_key"].(string))
	server.allowedHosts = strings.Split(result["server_allowed_hosts"].(string), ",")
	port := int(result["server_port"].(float64))

	if len(os.Getenv("API_WORK")) > 0 {
		server.allowedHosts = append(server.allowedHosts, "http://localhost:8082")
	}

	listenAddr := fmt.Sprintf(":%d", port)
	handler := http.HandlerFunc(makeHandler(routes))
	return http.ListenAndServe(listenAddr, handler)
}

func NewRoute(method Method, pattern string, handler http.HandlerFunc) Route {
	return Route{method, regexp.MustCompile("^" + pattern + "$"), handler}
}

func makeHandler(routes []Route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var allow []string

		origin := r.Header.Get("Origin")
		if !contains(server.allowedHosts, origin) {
			Respond(w, http.StatusForbidden, EMPTY, nil)
			return
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		for i, route := range routes {
			if route.method == LIST {
				routes[i].method = GET
				route.method = POST
				routes = append(routes, route)
			}
		}

		for _, route := range routes {
			matches := route.regex.FindStringSubmatch(r.URL.Path)

			if len(matches) > 0 {
				if r.Method == http.MethodOptions {
					// enable Cors
					w.Header().Set("Access-Control-Allow-Methods",
						"GET,POST,PATCH,PUT,DELETE,OPTIONS")

					w.Header().Set("Access-Control-Allow-Headers",
						"access-control-allow-origin,authorization,content-type,x-requested-with")

					w.Header().Set("Access-Control-Allow-Credentials", "true")

					w.Header().Set("Access-Control-Max-Age", "3600")

					w.WriteHeader(http.StatusNoContent)
					return
				}

				if r.Method != string(route.method) {
					allow = append(allow, string(route.method))
					continue
				}

				ctx := context.WithValue(r.Context(), ctxKey{}, matches[1:])
				route.handler(w, r.WithContext(ctx))
				return
			}
		}

		if len(allow) > 0 {
			w.Header().Set("Allow", strings.Join(allow, ", "))
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)

			return
		}

		http.NotFound(w, r)
	}
}

// ////////////////////////////////////////////////////////////////////////////
// Response helpers

func Respond(w http.ResponseWriter, status int, contentType ContentType, output any) {
	if contentType != EMPTY {
		w.Header().Set("Content-Type", string(contentType))
	}

	w.WriteHeader(status)

	if output != nil {
		json.NewEncoder(w).Encode(&output)
	}
}

func RespondIfFound(w http.ResponseWriter, contentType ContentType, output any) {
	if output != nil && !reflect.ValueOf(output).IsZero() {
		Respond(w, http.StatusOK, contentType, output)
		return
	}

	Respond(w, http.StatusNotFound, EMPTY, nil)
}

func ReadPayload(r *http.Request) Payload {
	payload := Payload{}
	decoder := json.NewDecoder(r.Body)
	decoder.Decode(&payload)

	return payload
}

func ReadEntity[T any](r *http.Request) T {
	var entity T
	decoder := json.NewDecoder(r.Body)
	decoder.Decode(&entity)

	return entity
}

func ReadQueryParam(r *http.Request, index int) string {
	fields := r.Context().Value(ctxKey{}).([]string)
	if len(fields) > index {
		return fields[index]
	}

	panic("Wrong route - no field")
}

// ////////////////////////////////////////////////////////////////////////////
// JWT

func Autorize(username string, role string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		USER:  username,
		ROLE:  role,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString(server.secretKey)
	if err != nil {
		return "Signing Error", err
	}

	return tokenString, nil
}

func Auth(r *http.Request) (string, string, error) {
	if r.Header["Authorization"] != nil && len(r.Header["Authorization"]) == 1 && strings.Contains(r.Header["Authorization"][0], "Bearer ") {
		bearer := strings.Split(r.Header["Authorization"][0], " ")[1]
		token, err := jwt.Parse(bearer, func(token *jwt.Token) (interface{}, error) {
			// Don't forget to validate the alg is what you expect:
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return server.secretKey, nil
		})

		if err != nil {
			return "", "", err
		}

		if token.Valid {
			claims, ok := token.Claims.(jwt.MapClaims)
			if ok {
				username := claims[USER].(string)
				role := claims[ROLE].(string)
				return username, role, nil
			}
		}
	}

	return "", "", errors.New("no token/error parsing token")
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
