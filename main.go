package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/magiconair/properties"
	"github.com/thoas/go-funk"
	"github.com/willianSteffler/libsrv/data"
	"github.com/willianSteffler/libsrv/socket"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
)

const (
	OP_CRIAR = "criar"
	OP_BUSCAR = "buscar"
	OP_REMOVER = "remover"
	OP_ALTERAR = "alterar"
	OP_ALTERAR_BUSCAR = "alterar_buscar"
	OPT_SAIR = "sair"
	OPT_CANCELAR = "cancelar"
)

var conf struct {
	interativo bool
	operacao string
}

var ctx struct {
	operacao string
	addrs string
	soc net.Conn
}

func main (){
	flag.StringVar(&conf.operacao, "operacao", "", "criar, consultar, alterar, remover")
	flag.BoolVar(&conf.interativo, "interativo", true, "usar modo interativo")


	Usage := func() {
		fmt.Fprintf(os.Stderr, "Sintaxe do comando %s: [opções] addrs\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "** addrs endereço do servidor\n")
		flag.PrintDefaults()
	}
	flag.Usage = Usage

	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	ctx.addrs = args[0]
	connectSocket()
	ctx.operacao = conf.operacao

	if conf.interativo{
		printHeader()
		p := prompt.New(
			executor,
			completer,
			prompt.OptionPrefix(ctx.operacao+"> "),
			prompt.OptionLivePrefix(livePrefix),
			prompt.OptionTitle("Biblioteca - Distribuidos"),
		)
		p.Run()
	} else {

	}
}

func livePrefix() (string, bool) {
	return ctx.operacao + "> ", true
}

func getLivroExato(in string ) (err error,livro *data.Livro){
	var c data.ConsultarLivroArgs
	if err,c = getConsulta(in); err != nil {
		return err,nil
	}

	c.Itens = 1
	c.Pagina = 0

	var msg data.SocketMsg

	msg.Operacao = socket.BUSCAR_LIVRO
	msg.OpArgs = &data.SocketMsg_Consulta{Consulta: &c}
	err,resp := sendReceiveMessage(msg)
	if err == nil {
		livros := resp.GetConsultaResp()
		if len(livros.Livros) == 0  {
			err = fmt.Errorf("Não foi encontrado nenhum livro para o título '%s'",c.Titulo)
		} else {
			livro = livros.Livros[0]
		}
	}

	return err,livro
}

func executor(in string) {
	msg := data.SocketMsg{}
	resp := data.SocketMsg{}
	in = strings.TrimSpace(in)
	var err error

	if in == OPT_CANCELAR || in == OPT_SAIR{
		ctx.operacao = in
	}

	switch ctx.operacao {
	case "":
		if funk.Contains(getSuggestionKeys(),in){
			ctx.operacao = in
		} else {
			fmt.Println("operação não encontrada !")
		}
	case OP_CRIAR:
		var l data.Livro
		if err,l = getLivro(in); err == nil{
			msg.Operacao = socket.CRIAR_LIVRO
			msg.OpArgs = &data.SocketMsg_Livro{Livro: &l}
			err,resp = sendReceiveMessage(msg)
			if err == nil {
				fmt.Printf("livro criado ! %s\n",prettyO(resp.GetLivro()))
			}
		}
	case OP_BUSCAR:
		var c data.ConsultarLivroArgs
		if err,c = getConsulta(in); err == nil{
			if c.Titulo != ""{
				c.Titulo = "%" + c.Titulo + "%"
			}
			if c.Nome != "" {
				c.Nome = "%" + c.Nome + "%"
			}
			msg.Operacao = socket.BUSCAR_LIVRO
			msg.OpArgs = &data.SocketMsg_Consulta{Consulta: &c}
			err,resp = sendReceiveMessage(msg)
			if err == nil  {
				fmt.Printf("%s\n",prettyO(resp.GetConsultaResp()))
			}
		}
	case OP_REMOVER:
		if err,l := getLivroExato(in); err == nil{
			msg.Operacao = socket.REMOVER_LIVRO
			msg.OpArgs = &data.SocketMsg_Livro{Livro: l}
			err,resp = sendReceiveMessage(msg)
			if err == nil {
				fmt.Printf("Objeto removido %s",prettyO(resp.GetLivro()))
			}
		}
	case OP_ALTERAR:
		if err,l := getLivroExato(in); err == nil {
			var updt data.Livro
			err, updt = getNovoLivro(in)
			if err == nil {
				updt.Codigo = l.Codigo
				msg.Operacao = socket.ALTERAR_LIVRO
				msg.OpArgs = &data.SocketMsg_Livro{Livro: &updt}
				err,resp = sendReceiveMessage(msg)
				if err == nil {
					fmt.Printf("Objeto alterado %s",prettyO(resp.GetLivro()))
				}
			}
		}
	case OPT_CANCELAR:
		ctx.operacao = ""
	case OPT_SAIR:
		os.Exit(0)
	}


	if err != nil {
		fmt.Println(fmt.Errorf("erro na operação %s : %v",ctx.operacao,err))
	}
	printHeader()
}

func prettyO(v interface{}) string{
	d, _ := json.MarshalIndent(v,"","  ")
	return string(d)
}

func parseProps(in string) (*properties.Properties,error){
	r := regexp.MustCompile("(\\w+)=\"([a-zA-Z0-9_\\s,áàâãéèêíïóôõöúçñÁÀÂÃÉÈÍÏÓÔÕÖÚÇÑ']+)\"")
	v:= r.FindAllString(in,-1)
	v = funk.Map(v,func (s string)string{
		s = strings.Replace(s,"\"","",1)
		return strings.TrimRight(s,"\"")
	}).([]string)

	return properties.LoadString(strings.Join(v,"\n"))
}


func getConsulta(in string ) (error,data.ConsultarLivroArgs){
	var c = data.ConsultarLivroArgs{}

	p,err := parseProps(in)
	if err == nil {
		c.Titulo = p.GetString("titulo","")
		c.Nome = p.GetString("autores","")
		c.Edicao = int32(p.GetInt("edicao",0))
		c.Ano = int32(p.GetInt("ano",0))
		c.Pagina = int32(p.GetInt("pagina",0))
		c.Itens = int32(p.GetInt("itens",10))
	}


	return err,c
}

func getLivro(in string) (error,data.Livro){
	var livro = data.Livro{}

	err,props := getConsulta(in)
	if err == nil {
		vautores := strings.Split(props.Nome,",")
		livro.Titulo = props.Titulo
		if props.Edicao != 0 || props.Ano != 0 {

			livro.Edicoes = []*data.Edicao{{
				Numero: props.Edicao,
				Ano: props.Ano,
			}}
		}

		if len(vautores) > 1 ||  len(vautores) == 1 && vautores[0] != "" {
			for _, nome := range vautores {
				livro.Autores = append(livro.Autores, &data.Autor{
					Nome: nome,
				})
			}
		}
	}

	return err,livro
}

func getNovoLivro(in string) (error,data.Livro){
	var livro = data.Livro{}

	p,err := parseProps(in)
	if err == nil {
		livro.Titulo = p.GetString("novotitulo","")
		vautores := strings.Split(p.GetString("novosautores",""),",")
		edicao := int32(p.GetInt("novaedicao",0))
		ano := int32(p.GetInt("novoano",0))

		if edicao != 0 || ano != 0 {

			livro.Edicoes = []*data.Edicao{{
				Numero: edicao,
				Ano: ano,
			}}
		}

		if len(vautores) > 1 ||  len(vautores) == 1 && vautores[0] != ""{
			for _, nome := range vautores {
				livro.Autores = append(livro.Autores, &data.Autor{
					Nome:   nome,
				})
			}
		}

	}

	return err,livro
}


func getSuggestions() []prompt.Suggest{
	var suggestions []prompt.Suggest
	switch ctx.operacao {
	case "":
		suggestions = []prompt.Suggest{
			{OP_CRIAR,"criar livro na base de dados"},
			{OP_BUSCAR,"buscar livro na base de dados"},
			{OP_REMOVER,"remover livro da base de dados"},
			{OP_ALTERAR,"alterar livro na base de dados"},
			{OPT_SAIR,"sair do programa"},
		}

	case OP_CRIAR,OP_BUSCAR,OP_ALTERAR:
		suggestions = []prompt.Suggest{
			{"titulo=\"","titulo do livro titulo do livro "},
			{"autores=\"","autores do livro separado por ,"},
			{"edicao=\"","edicao do livro"},
			{"ano=\"","ano do livro"},
			{OPT_CANCELAR,"cancelar operação"},
			{OPT_SAIR,"sair do programa"},
		}

		if ctx.operacao == OP_BUSCAR {
			suggestions = append(suggestions, []prompt.Suggest{
				{"itens=\"","numero maximo de itens"},
				{"pagina=\"","pagina de itens"},
			}...)
		}

		if ctx.operacao == OP_ALTERAR {
			suggestions = append(suggestions, []prompt.Suggest{
				{"novotitulo=\"","novo titulo do livro"},
				{"novosautores=\"","novos autores do livro separado por ,"},
				{"novaedicao=\"","nova edicao do livro"},
				{"novoano=\"","novo ano do livro"},
			}...)
		}

	case OP_REMOVER:
		suggestions = []prompt.Suggest{
			{"titulo=\"", "titulo do livro"},
			{OPT_CANCELAR,"cancelar operação"},
			{OPT_SAIR,"sair do programa"},
		}

	}

	return suggestions
}

func getSuggestionKeys() []string {
	return funk.Map(getSuggestions(),func(suggest prompt.Suggest)string{return suggest.Text}).([]string)
}

func printHeader(){
	switch ctx.operacao {
	case "":
		fmt.Printf("digite uma operação %v\n",getSuggestionKeys())
	case OP_CRIAR:
		fmt.Printf("digite as propriedades do livro no formato prop=\"val\"  %v\n",funk.Without(getSuggestionKeys(),"cancelar","sair"))
	case OP_BUSCAR:
		fmt.Printf("digite as propriedades da busca no formato prop=\"val\"  %v\n",funk.Without(getSuggestionKeys(),"cancelar","sair"))
	case OP_ALTERAR:
		fmt.Printf("digite as informações completas as novas propriedades do livro no formato prop=\"val\"  %v\n",funk.Without(getSuggestionKeys(),"cancelar","sair"))
	case OP_REMOVER:
		fmt.Printf("digite o titulo do livro no formato prop=\"val\"  %v\n",funk.Without(getSuggestionKeys(),"cancelar","sair"))
	}
}

func completer(in prompt.Document) []prompt.Suggest {
	w := in.GetWordBeforeCursor()
	if w == "" {
		return []prompt.Suggest{}
	}

	return prompt.FilterHasPrefix(getSuggestions(), w, true)
}

func sendReceiveMessage(msg data.SocketMsg) (error,data.SocketMsg){
	sendMessage(msg)
	var err error
	m := receiveMessage()
	if e:=  m.GetErro(); e != nil {
		err = fmt.Errorf("%s",e.Erro)
	}
	return err,m
}

func sendMessage(msg data.SocketMsg) {
	if err := socket.SendSocketMessage(ctx.soc,msg); err != nil {
		log.Print(fmt.Errorf("erro ao enviar mensagem ao socket %v\n",err))
		reconnectSocket()
	}
}

func receiveMessage() data.SocketMsg{
	var err error
	var msg data.SocketMsg
	if err,msg = socket.GetSocketMessage(ctx.soc); err != nil {
		log.Print(fmt.Errorf("erro ao enviar mensagem ao socket %v\n",err))
		reconnectSocket()
	}
	return msg
}

func connectSocket(){
	log.Printf("conectando socket no endereço %s\n",ctx.addrs)
	conn, err := net.Dial("tcp", ctx.addrs)
	if err != nil{
		log.Fatalf("erro ao abrir conexão socket %v",err)
	}
	ctx.soc = conn
}

func reconnectSocket(){
	log.Printf("fechando conexão com socket %s\n",ctx.soc.RemoteAddr().String())
	ctx.soc.Close()
	connectSocket()
}
