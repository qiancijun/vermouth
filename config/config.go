package config

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/qiancijun/vermouth/utils"
	"github.com/spf13/viper"
)

const (
	CLUSTER  = "cluster"
	STAND    = "stand"
	REQUIRED = "required"
	FILEPATH = "filePath"
	ENUM     = "enum"
)

var (
	ErrorUnknownTag         = errors.New("field with unsupported tag")
	ErrorIllegalConfigFile  = errors.New("illegal config, please check toml file")
	ErrorEnumFieldsTooShort = errors.New("enum fields too short")
	ErrorCannotCatchAnyEnum = errors.New("can not catch any enum field")
)

type VermouthConfig struct {
	Server    ServerArgs
	HttpProxy []HttpProxyArgs
	Discovery []Discovery
	Raft      RaftArgs
	Log       []LogArgs
}

type ServerArgs struct {
	HttpServer bool
	HttpPort   int64 `vaild:"required"`
	RpcServer  bool
	RpcPort    int64  `vaild:"required"`
	Name       string `vaild:"required"`
	Mode       string `vaild:"required, enum:stand_cluster"`
	RootLevel  string `toml:"root_level"`
	DataDir    string `vaild:"required, filePath"`
}

type HttpProxyArgs struct {
	Port         int64  `vaild:"required"`
	PrefixerType string `vaild:"required"`
	Paths        []HttpProxyPathArgs
}

type HttpProxyPathArgs struct {
	BalanceMode string `vaild:"required"`
	Pattern     string `vaild:"required"`
	Static      bool
	Hosts       []string
}

type Discovery struct {
	Port           int64  `vaild:"required"`
	Namespace      string `vaild:"required, enum:zookeeper"`
	Address        []string
	ConnectTimeout int64 `vaild:"required"`
}

type RaftArgs struct {
	TcpAddress        string
	Leader            bool
	Method            string `enum:"http_rpc"`
	LeaderAddress     string
	SnapshotInterval  int
	SnapshotThreshold int
	HeartbeatTimeout  int
	ElectionTimeout   int
}

type LogArgs struct {
	Name      string
	Type      string
	Level     string
	Color     bool
	Format    string
	FileName  string
	MaxSize   int64
	MaxLine   int64
	DateSlice string
}

func NewVermouthConfig() *VermouthConfig {
	return &VermouthConfig{}
}

// TODO path 可以通过命令行指定
func (c *VermouthConfig) ReadConfig(path string) error {
	dir, base := filepath.Dir(path), filepath.Base(path)
	viper.AddConfigPath(dir)
	viper.SetConfigName(base)
	viper.SetConfigType("toml")
	if err := viper.ReadInConfig(); err != nil {
		return err
	}
	if err := viper.Unmarshal(c); err != nil {
		return err
	}
	return c.VaildConfig()
}

// TODO 可能是正确的
func (c *VermouthConfig) VaildConfig() error {
	ok, err := c.checkArg(reflect.ValueOf(c).Elem())
	if err != nil {
		return err
	}
	if !ok {
		return ErrorIllegalConfigFile
	}
	return nil
}

func (c *VermouthConfig) checkArg(arg reflect.Value) (bool, error) {
	t := arg.Type()
	// 获取该配置参数对应结构体的 tag
	for i := 0; i < t.NumField(); i++ {
		// 如果是结构体递归检查一下
		field := t.Field(i)
		t := field.Type.Kind()
		if t == reflect.Struct {
			ok, err := c.checkArg(reflect.ValueOf(field))
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
			continue
		}
		tag := field.Tag.Get("vaild")
		if tag == "" {
			continue
		}
		val := arg.Field(i)
		rules := utils.ParseTag(tag)
		for _, rule := range rules {
			ok, err := c.checkSingleTag(val, rule)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
	}
	return true, nil
}

func (c *VermouthConfig) checkSingleTag(val reflect.Value, rule string) (bool, error) {
	t := val.Type()
	switch t.Kind() {
	case reflect.String:
		copyVal := val.String()
		return c.checkString(copyVal, rule)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		copyVal := val.Int()
		return c.checkInt(copyVal, rule)
	}
	return true, nil
}

func (c *VermouthConfig) checkString(val, rule string) (bool, error) {
	switch {
	case rule == REQUIRED:
		// 必须提供，不能是空字符串
		if val == "" {
			return false, nil
		}
	case rule == FILEPATH:
		// 必须是一个文件路径
		ok := utils.PathIsValid(val)
		return ok, nil
	case strings.Contains(rule, ENUM):
		// 解析出枚举字段
		splits := strings.Split(rule, ":")
		if len(splits) < 2 {
			return false, ErrorEnumFieldsTooShort
		}
		enums := strings.Split(splits[1], "_")
		for _, enum := range enums {
			if val == enum {
				return true, nil
			}
		}
		return false, ErrorCannotCatchAnyEnum
	default:
		return false, ErrorUnknownTag
	}
	return true, nil
}

func (c *VermouthConfig) checkInt(val int64, rule string) (bool, error) {
	switch rule {
	case REQUIRED:
		if val == 0 {
			return false, nil
		}
	default:
		return false, ErrorUnknownTag
	}
	return true, nil
}
