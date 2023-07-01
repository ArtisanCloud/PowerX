# 本地打包PowerX的Docker镜像


```shell
# 按照你的环境和需求，可替换{xxx}里的值

# 进入你的项目文件根目录下
> cd {your_project_path}

# 编译项目中deploy/docker/Dockerfile
> docker build -t {powerx}:{latest} -f ./deploy/docker/Dockerfile .

# 编译完后的镜像，直接用docker可以跑起来
> docker run -p {8888}:{8888} -it {powerx-dashboard}:{latest}

```

# 备份数据库脚本

使用该脚本时，请确保替换以下配置项：

DB_HOST：数据库的主机名或 IP 地址。  
DB_PORT：数据库的端口号。  
DB_NAME：要备份的数据库名称。  
DB_USER：用于连接数据库的用户名。  
DB_PASSWORD：用于连接数据库的密码。  
BACKUP_DIR：备份文件保存的目录路径。  

你可以将脚本保存为 .sh 文件，并设置为可执行文件，例如 backup.sh。
然后，可以使用 cron 等工具在指定的时间间隔内运行该脚本。
例如，可以在 crontab 文件中添加以下条目以每天凌晨 2 点运行备份：

```shell
0 2 * * * /path/to/backup.sh

```