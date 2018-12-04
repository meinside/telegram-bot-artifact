package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	a "github.com/meinside/steam-community-market-artifact"
	t "github.com/meinside/telegram-bot-go"
)

const (
	// config filename
	confFilename = "./config.json"

	// cache ttl
	cacheMinutes = 5

	// commands
	commandStart     = "/start"
	commandSummarize = "/summarize"
	commandHelp      = "/help"

	// messages
	messageUnknownCommand = "Unknown command"
	messageHelpEng        = `*Help:*

This is a Telegram bot which fetches information of *Artifact* from _Steam Community Market_.

Supported commands are as following:

%s: Summarize current market information.
%s: Show this help message.

You can search for card info in other chats with:

*@%s [search keyword]*
`
	messageHelpKor = `*도움말:*

_스팀 커뮤니티 장터_에서 *Artifact* 정보를 가져오는 Telegram Bot입니다.

지원되는 명령어는 다음과 같습니다:

%s: 현재 장터 정보를 요약합니다.
%s: 이 도움말을 표시합니다.

다른 대화창에서

*@%s [검색어]*

를 입력하여 카드 정보를 바로 조회, 인용할 수 있습니다.
`
	messageSummaryEng = `*Summary:*

Number of all items: %d
All %d commons (%d cards): *$%.2f*
All %d uncommons (%d cards): *$%.2f*
All %d rares (%d cards): *$%.2f*
----
Price for full collection: *$%.2f* (+ tax/fee $%.2f = *$%.2f*)

_last update: %s_
`
	messageSummaryKor = `*요약:*

모든 항목: %d종
모든 일반 카드 %d종 (%d 장): *$%.2f*
모든 고급 카드 %d종 (%d 장): *$%.2f*
모든 희귀 카드 %d종 (%d 장): *$%.2f*
----
풀 컬렉션 수집 비용: *$%.2f* (+ 세금/수수료 $%.2f = *$%.2f*)

_마지막 갱신: %s_
`
)

const (
	maxNumCardsPerDeck     = 3
	maxNumHeroCardsPerDeck = 1
)

// config struct
type config struct {
	Token                  string `json:"token"`                    // Telegram bot token
	MonitorIntervalSeconds int    `json:"monitor_interval_seconds"` // polling interval seconds
	Verbose                bool   `json:"verbose"`                  // show verbose logs or not
}

var _conf config
var _botName string
var _lock sync.RWMutex
var _items map[a.Lang][]a.MarketItem   // market items
var _itemsUpdated map[a.Lang]time.Time // times when market items were updated successfully

// localized constants
var _localizedHeroes map[a.Lang][]string
var _localizedRarities map[a.Lang]map[a.Rarity]string

// initialize things
func init() {
	_conf = readConfig()
	_lock = sync.RWMutex{}
	_items = map[a.Lang][]a.MarketItem{}
	_itemsUpdated = map[a.Lang]time.Time{}

	// localized variables
	_localizedHeroes = map[a.Lang][]string{
		a.LangEnglish: []string{
			"Axe",
			"Bristleback",
			"Drow Ranger",
			"Kanna",
			"Lich",
			"Tinker",
			"Legion Commander",
			"Lycan",
			"Phantom Assassin",
			"Omniknight",
			"Luna",
			"Bounty Hunter",
			"Ogre Magi",
			"Sniper",
			"Treant Protector",
			"Beastmaster",
			"Enchantress",
			"Sorla Khan",
			"Chen",
			"Zeus",
			"Ursa",
			"Skywrath Mage",
			"Winter Wyvern",
			"Venomancer",
			"Prellex",
			"Earthshaker",
			"Magnus",
			"Sven",
			"Dark Seer",
			"Debbi the Cunning", // basic
			"Mazzie",
			"J'Muy the Wise",       // basic
			"Fahrvhan the Dreamer", // basic
			"Necrophos",
			"Centaur Warrunner",
			"Abaddon",
			"Viper",
			"Timbersaw",
			"Keefe the Bold", // basic
			"Tidehunter",
			"Crystal Maiden",
			"Bloodseeker",
			"Pugna",
			"Lion",
			"Storm Spirit",
			"Meepo",
			"Rix",
			"Outworld Devourer",
			// TODO - add more heroes here
		},
		a.LangKorean: []string{
			"도끼전사",
			"가시멧돼지",
			"드로우 레인저",
			"칸나",
			"리치",
			"땜장이",
			"군단 사령관",
			"늑대인간",
			"유령 자객",
			"전능기사",
			"루나",
			"현상금 사냥꾼",
			"오거 마법사",
			"저격수",
			"나무정령 수호자",
			"야수지배자",
			"요술사",
			"솔라 칸",
			"첸",
			"제우스",
			"우르사",
			"하늘분노 마법사",
			"겨울 비룡",
			"맹독사",
			"프렐렉스",
			"지진술사",
			"마그누스",
			"스벤",
			"어둠 현자",
			"교활한 데비", // basic
			"매지",
			"현자 제이무이",              // basic
			"Fahrvhan the Dreamer", // basic
			"강령사제",
			"켄타우로스 전쟁용사",
			"아바돈",
			"바이퍼",
			"벌목꾼",
			"Keefe the Bold", // basic
			"파도사냥꾼",
			"수정의 여인",
			"혈귀",
			"퍼그나",
			"라이온",
			"폭풍령",
			"미포",
			"릭스",
			"외계 침략자",
			// TODO - add more heroes here
		},
		// TODO - add more localizations here
	}

	_localizedRarities = map[a.Lang]map[a.Rarity]string{
		a.LangEnglish: map[a.Rarity]string{
			a.RarityCommon:   "Common Card",
			a.RarityUncommon: "Uncommon Card",
			a.RarityRare:     "Rare Card",
		},
		a.LangKorean: map[a.Rarity]string{
			a.RarityCommon:   "일반 카드",
			a.RarityUncommon: "고급 카드",
			a.RarityRare:     "희귀 카드",
		},
		// TODO - add more localizations here
	}
}

// read config file
func readConfig() config {
	_, filename, _, _ := runtime.Caller(0) // = __FILE__

	var file []byte
	var err error
	file, err = ioutil.ReadFile(filepath.Join(path.Dir(filename), confFilename))
	if err == nil {
		var conf config
		if err = json.Unmarshal(file, &conf); err == nil {
			return conf
		}
	}

	panic(err)
}

// get help message
func getHelp(language a.Lang) string {
	if language == a.LangKorean {
		return fmt.Sprintf(messageHelpKor, commandSummarize, commandHelp, _botName)
	}

	// default = English
	return fmt.Sprintf(messageHelpEng, commandSummarize, commandHelp, _botName)
}

// get message options
func getMessageOptions() t.OptionsSendMessage {
	return t.OptionsSendMessage{}.
		SetReplyMarkup(t.ReplyKeyboardMarkup{
			Keyboard: [][]t.KeyboardButton{
				t.NewKeyboardButtons(commandSummarize, commandHelp),
			},
			ResizeKeyboard: true,
		}).
		SetParseMode(t.ParseModeMarkdown)
}

// get items
func getItems(language a.Lang) []a.MarketItem {
	_lock.Lock()
	defer _lock.Unlock()

	needsReload := false

	// check last updated time,
	if updated, exists := _itemsUpdated[language]; exists {
		// if it is outdated,
		if updated.Add(cacheMinutes * time.Minute).Before(time.Now()) {
			needsReload = true
		}
	} else {
		needsReload = true
	}

	// reload,
	if needsReload {
		items, err := a.FetchAll(a.RarityAll, language, a.SortColumnName, a.SortDirectionAsc)

		if err == nil {
			// update values
			_items[language] = items
			_itemsUpdated[language] = time.Now()

			return items
		}

		log.Printf("Failed to reload items (%s): %s", language, err)
	} else {
		// return cached items
		return _items[language]
	}

	// return empty slice on error
	return []a.MarketItem{}
}

// get market summary
func getSummary(language a.Lang) string {
	var numItems,
		numCommons, numCommonCards, priceCommons,
		numUncommons, numUncommonCards, priceUncommons,
		numRares, numRareCards, priceRares int

	// calculate values
	for _, item := range getItems(language) {
		numItems++

		// number of cards per item
		numCards := maxNumCardsPerDeck
		if isHero(item.Name, language) {
			numCards = maxNumHeroCardsPerDeck
		}

		// check rarity
		switch rarityOf(item, language) {
		case a.RarityCommon:
			numCommons++
			numCommonCards += numCards
			priceCommons += item.SellPrice * numCards
		case a.RarityUncommon:
			numUncommons++
			numUncommonCards += numCards
			priceUncommons += item.SellPrice * numCards
		case a.RarityRare:
			numRares++
			numRareCards += numCards
			priceRares += item.SellPrice * numCards
		}
	}

	total := float32(priceCommons+priceUncommons+priceRares) / 100.0
	tax := taxOf(total)

	// last updated time
	_lock.RLock()
	var lastUpdated time.Time
	var exists bool
	if lastUpdated, exists = _itemsUpdated[language]; !exists {
		lastUpdated = time.Time{}
	}
	_lock.RUnlock()

	// localized summary format
	summary := messageSummaryEng
	if language == a.LangKorean {
		summary = messageSummaryKor
	}

	return fmt.Sprintf(summary,
		numItems,
		numCommons, numCommonCards, float32(priceCommons)/100.0,
		numUncommons, numUncommonCards, float32(priceUncommons)/100.0,
		numRares, numRareCards, float32(priceRares)/100.0,
		total, tax, total+tax,
		lastUpdated.Format("2006-01-02 (Mon) 15:04:05"),
	)
}

// search items by name (ignore case)
func searchItemsByName(name string, language a.Lang) []a.MarketItem {
	results := []a.MarketItem{}

	for _, item := range getItems(language) {
		if strings.Contains(strings.ToLower(item.Name), strings.ToLower(name)) {
			results = append(results, item)
		}
	}

	return results
}

// check language from given Telegram user
func langFromUser(u *t.User) a.Lang {
	if u != nil {
		langCode := u.LanguageCode

		if langCode != nil {
			if strings.HasPrefix(*langCode, "ko") {
				return a.LangKorean
			}
			// TODO - add more languages here
		}
	}

	return a.LangEnglish // default
}

// check if a card with given name is a hero
func isHero(name string, language a.Lang) bool {
	if _, exists := _localizedHeroes[language]; !exists {
		log.Printf("* No heroes defined for language: %s", language)

		return false
	}

	for _, hero := range _localizedHeroes[language] {
		if hero == name {
			return true
		}
	}

	return false
}

// get rarity of given item
func rarityOf(item a.MarketItem, language a.Lang) a.Rarity {
	itemType := item.AssetDescription.Type
	rarities := _localizedRarities[language]

	for k, v := range rarities {
		if itemType == v {
			return k
		}
	}

	return a.RarityAll // unknown rarity
}

// calculate tax of given price
func taxOf(price float32) float32 {
	return 0.15 * price
}

// process incoming updates with this function
func processUpdate(b *t.Bot, update t.Update) bool {
	// process result
	result := false

	// text from message
	var txt string
	if update.Message.HasText() {
		txt = *update.Message.Text
	} else {
		txt = ""
	}

	language := langFromUser(update.Message.From)

	var message string

	switch {
	// start
	case strings.HasPrefix(txt, commandStart):
		message = getHelp(language)
		// summarize
	case strings.HasPrefix(txt, commandSummarize):
		message = getSummary(language)
	// help
	case strings.HasPrefix(txt, commandHelp):
		message = getHelp(language)
	// fallback
	default:
		if len(txt) > 0 {
			message = fmt.Sprintf("*%s*: %s", txt, messageUnknownCommand)
		} else {
			message = messageUnknownCommand
		}
	}

	if len(message) > 0 {
		// 'typing...'
		b.SendChatAction(update.Message.Chat.ID, t.ChatActionTyping)

		// send message
		if sent := b.SendMessage(update.Message.Chat.ID, message, getMessageOptions()); sent.Ok {
			result = true
		} else {
			log.Printf("Failed to send message: %s", *sent.Description)
		}
	}

	return result
}

// process inline query
func processInlineQuery(b *t.Bot, update t.Update) bool {
	language := langFromUser(&update.InlineQuery.From)

	// query length limit differs between languages
	queryLengthLimit := 3
	if language == a.LangKorean {
		queryLengthLimit = 1
	}
	// TODO - add more length limits for different languages

	query := strings.TrimSpace(update.InlineQuery.Query)

	// when query is too short,
	if len(query) < queryLengthLimit {
		return false
	}

	// search with given query,
	searchedItems := searchItemsByName(query, language)

	if len(searchedItems) > 0 {
		itemResults := []interface{}{}

		// build up inline query results,
		for _, item := range searchedItems {
			url := item.StoreURL()
			thumbURL := item.AssetDescription.IconURL()

			message := fmt.Sprintf("%s (%s)\n%s\n%s", item.Name, item.AssetDescription.Type, item.SellPriceText, url)
			description := fmt.Sprintf("%s, %s, %s", item.Name, item.AssetDescription.Type, item.SellPriceText)

			if article, id := t.NewInlineQueryResultArticle(item.Name, message, description); id != nil {
				article.URL = &url
				article.ThumbURL = &thumbURL

				itemResults = append(itemResults, article)
			}
		}

		// then answer inline query
		sent := b.AnswerInlineQuery(
			update.InlineQuery.ID,
			itemResults,
			nil,
		)

		if sent.Ok {
			return true
		}

		log.Printf("Failed to answer inline query: %s", *sent.Description)
	} else {
		log.Printf("No matching item with name: %s", query)
	}

	return false
}

func main() {
	bot := t.NewClient(_conf.Token)
	bot.Verbose = _conf.Verbose

	if me := bot.GetMe(); me.Ok {
		log.Printf("Starting bot: @%s (%s)\n", *me.Result.Username, me.Result.FirstName)

		// save bot name
		_botName = *me.Result.Username

		// delete webhook first
		unhooked := bot.DeleteWebhook()
		if unhooked.Ok {
			// wait for new updates
			bot.StartMonitoringUpdates(0, _conf.MonitorIntervalSeconds, func(b *t.Bot, update t.Update, err error) {
				if err == nil {
					if update.HasMessage() {
						processUpdate(b, update)
					} else if update.HasInlineQuery() {
						processInlineQuery(b, update)
					}
				} else {
					log.Printf("Error while receiving update (%s)", err.Error())
				}
			})
		} else {
			panic("Failed to delete webhook")
		}
	} else {
		panic("Failed to get info of this bot")
	}
}
