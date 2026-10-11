package main
import (
 "context"
 "encoding/json"
 "net"
 "net/http"
 "net/url"
 "os"
 "os/signal"
 "syscall"
 
 "time"
 "github.com/ArtisanCloud/PowerX/config"
 "github.com/ArtisanCloud/PowerX/internal/app/shared"
 "github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
 openai "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/ai"
 "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
 "github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
 coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
 "github.com/gin-gonic/gin"
 
)
func main(){
 e:=config.InitGlobalConfig("etc/config.yaml");if e!=nil{panic("DIRECTOR_CORE_CONFIG_INVALID")};cfg:=config.GetGlobalConfig();if _,e=cfg.Server.ParseKey();e!=nil{panic("DIRECTOR_MASTER_KEY_INVALID")}
 // Explicit acceptance target: current host credentials, original database.
 d:=cfg.Database;u,e:=url.Parse(d.DSN);if e!=nil||u.Port()!="5432"{panic("DIRECTOR_ORIGINAL_DATABASE_TARGET_INVALID")};u.Path="/powerx";d.DSN=u.String();d.Database="powerx";cfg.Database=d
 if e:=catalog.InitFromAppConfig(cfg.AI.Catalog,nil);e!=nil{panic("DIRECTOR_MODEL_CATALOG_INIT_FAILED")}
 coremodel.PowerXSchema="public"
 db,e:=database.Connect(cfg.Database);if e!=nil{panic("DIRECTOR_ORIGINAL_DATABASE_CONNECT_FAILED")};sqlDB,e:=db.DB();if e!=nil{panic("DIRECTOR_ORIGINAL_DATABASE_HANDLE_FAILED")};defer sqlDB.Close()
 gin.SetMode(gin.ReleaseMode);r:=gin.New();r.Use(gin.Recovery());protected:=r.Group("/api/v1");protected.Use(middleware.APIKeyOrJwtMiddleware(db,[]byte(cfg.Auth.JWTSecret),cfg.Auth.Issuer,[]string{cfg.Auth.AudienceUser},[]string{"access"},nil));openai.Register(nil,protected,&shared.Deps{DB:db})
 listener,e:=net.Listen("tcp","127.0.0.1:0");if e!=nil{panic("DIRECTOR_ACCEPTANCE_LISTENER_FAILED")};state:=map[string]any{"pid":os.Getpid(),"cwd":"/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/backend","command":"go run ./cmd/director-original-db-gateway-tmp","origin":"http://"+listener.Addr().String(),"database":d.Database};stateJSON,_:=json.Marshal(state);if os.WriteFile(os.Getenv("MEDIAX_DIRECTOR_ACCEPTANCE_STATE_PATH"),stateJSON,0600)!=nil{panic("DIRECTOR_ACCEPTANCE_STATE_FAILED")}
 server:=&http.Server{Handler:r,ReadHeaderTimeout:5*time.Second};go server.Serve(listener);ctx,cancel:=signal.NotifyContext(context.Background(),syscall.SIGTERM,syscall.SIGINT);defer cancel();<-ctx.Done();stopCtx,stop:=context.WithTimeout(context.Background(),5*time.Second);defer stop();server.Shutdown(stopCtx)
}
