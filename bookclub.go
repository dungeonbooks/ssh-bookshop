package main

// catalog is the book club shelf and the whole shop: ssh in, see what we're
// reading. Two clubs, each newest first: the monthly Sci-Fi & Fantasy club
// (which ran as the plain "Monthly Book Club" until mid 2025), then the Horror
// club, which started in October 2025. The Sci-Fi & Fantasy club comes first
// because featured() opens the shop on the top entry.
//
// This is the one hand-edited copy of the list. taranat.com renders its shelves
// from the API built on it, so a pick added here reaches both.
//
// ISBN is the edition we stock, so a cover lookup matches what is on the shelf.
// URL is where a reader should buy it: dungeonbooks.com while we have it, a
// bookshop.org affiliate link once we sell out.
var catalog = []Book{
	{ISBN: "9798893311891", BookTitle: "MAYA: Seed Takes Root", Author: "Anand Gandhi and Zain Memon", Collection: collBookClub, Month: "2026-10",
		Format: "hardcover", Pages: 416, WeightGrams: 771,
		Blurb: "A world where the internet grows from trees, and whoever tends them shapes what billions of people want. One boy never logged in.",
		URL:   "https://www.dungeonbooks.com/product/maya-seed-takes-root-hardcover-/6IV4EZ3QMYPN6TLCLAT3K5KC"},
	{ISBN: "9780316589710", BookTitle: "Harbour of Hungry Ghosts", Author: "Eliza Chan", Collection: collBookClub, Month: "2026-09",
		Format: "paperback", Pages: 400, WeightGrams: 386,
		Blurb: "The Au family keeps Hong Kong's shrines blessed and its monsters contained. When the British disrupt the Hungry Ghost festival and a creature nobody recognises carries off her grandmother, eldest daughter Kiamling leads the search with a civil servant, a childhood sweetheart turned pirate, and a sister keeping secrets of her own. First of the Chronicles of the Yiugwai Hunters.",
		URL:   "https://www.dungeonbooks.com/product/harbour-of-hungry-ghosts-chronicles-of-the-yiugwai-hunters-1-by-eliza-chan-paperback-/76IN4SA2PAKGRNSEQFLMLXOI"},
	{ISBN: "9780316568654", BookTitle: "The Last Contract of Isako", Author: "Fonda Lee", Collection: collBookClub, Month: "2026-08",
		Format: "paperback", Pages: 528, WeightGrams: 576,
		Blurb: "Isako is a legendary swordswoman planning to walk out into the ice and die well. The last contract she takes puts her opposite Martim, the worst apprentice she ever trained, who has climbed to the top of a company that can buy a life or end one.",
		URL:   "https://www.dungeonbooks.com/product/the-last-contract-of-isako-by-fonda-lee-paperback-/GB5UWQBUBNLLCSREFBVH3636"},
	{ISBN: "9780593818947", BookTitle: "Daughter of Crows", Author: "Mark Lawrence", Collection: collBookClub, Month: "2026-07",
		Format: "hardcover", Pages: 416, WeightGrams: 553,
		Blurb: "The Academy of Kindness takes in a hundred girls a year and graduates three. One survivor has to exhume her own past. First in a new series from the author of the Broken Empire.",
		URL:   "https://www.dungeonbooks.com/product/daughter-of-crows-book-1-of-the-academy-of-kindness-by-mark-lawrence-hardcover-/UEY72ZWABXCFBC7HZYKSMDFZ"},
	{ISBN: "9781250376794", BookTitle: "Sublimation", Author: "Isabel J. Kim", Collection: collBookClub, Month: "2026-06",
		Format: "hardcover, signed", Pages: 368, WeightGrams: 567,
		Blurb: "When you emigrate, a copy of you stays behind. Rose left Korea at ten and never spoke to hers again, until a funeral calls her home and that other self decides to take her life back.",
		URL:   "https://www.dungeonbooks.com/product/sublimation-by-isabel-j-kim-hardcover-signed/3HRN5W3LAQUUIILNTU77D5QU"},
	{ISBN: "9781967967063", BookTitle: "Burn the Sea", Author: "Mona Tewari", Collection: collBookClub, Month: "2026-05",
		// Page count off the physical copy: neither Hardcover nor Ingram has one.
		Format: "paperback", Pages: 450, WeightGrams: 567,
		Blurb: "Abbakka trained to be her sister's blade, not to rule. When the Porcugi come back out of the sea demanding tribute, she has to learn statecraft and subterfuge in a hurry.",
		URL:   "https://www.dungeonbooks.com/product/burn-the-sea-by-mona-tewari-paperback-/6BLH3723VWADJVHV3XMDOEPG"},
	{ISBN: "9798991475273", BookTitle: "Blessed Is the Rot", Author: "Sheri Singerling", Collection: collBookClub, Month: "2026-04",
		Format: "paperback", Pages: 338, WeightGrams: 390,
		Blurb: "Ashtin contained distortions for the Church until he defied it, lost his name, and became Fenrir, a bell ringer. When a distortion swallows his tower, the Church sends a surveyor to contain it.",
		URL:   "https://www.dungeonbooks.com/product/blessed-is-the-rot-by-sheri-singerling-paperback-/5XCYQAWHIVKO4CA3PFLFNQIA"},
	// The deluxe is the edition we stocked, so it is what Square prices and what
	// Bookshop is asked for. taranat.com swaps in the ebook's cover, because the
	// deluxe's art is a 3/4 render; no covers here, so no conflict.
	{ISBN: "9781250406811",
		BookTitle: "The Poet Empress", Author: "Shen Tao", Collection: collBookClub, Month: "2026-03",
		Format: "deluxe edition", Pages: 400, WeightGrams: 748,
		Blurb: "Wei Yin offers herself as concubine to a cruel prince to keep her family alive, and lands in a palace on the edge of civil war. To survive she becomes a poet, in a world where women are forbidden to read.",
		URL:   "https://www.dungeonbooks.com/product/the-poet-empress-by-shen-tao-deluxe-edition-/DGDHKCSBKDW7DPVVJPUPZIX4"},
	{ISBN: "9781250380968", BookTitle: "Saltcrop", Author: "Yume Kitasei", Collection: collBookClub, Month: "2026-02",
		Format: "hardcover", Pages: 384, WeightGrams: 581,
		Blurb: "The seas have swallowed the coastal cities and crops are failing worldwide. Two sisters sail out to find the third, who left a decade ago chasing a cure.",
		URL:   "https://www.dungeonbooks.com/product/saltcrop-by-yume-kitasei-hardcover-/SHDDSGUBEE7B2NOCEABTYGVK"},
	{ISBN: "9781984820716", BookTitle: "The Tainted Cup", Author: "Robert Jackson Bennett", Collection: collBookClub, Month: "2026-01",
		Format: "paperback", Pages: 432, WeightGrams: 318,
		Blurb: "An impossible death on the Empire's frontier, where leviathan blood warps everything it touches. The detective Ana Dolabra solves it from inside her house, blindfolded, through her altered assistant Din.",
		URL:   "https://www.dungeonbooks.com/product/the-tainted-cup-book-1-of-3-ana-and-din-mysteries-by-robert-jackson-bennett-paperback-/554"},
	{ISBN: "9781668083178", BookTitle: "All That We See or Seem", Author: "Ken Liu", Collection: collBookClub, Month: "2025-12",
		Format: "hardcover", Pages: 416, WeightGrams: 567,
		Blurb: "Julia Z, a former orphan hacker who can talk to AIs, is pulled back in when a dream artist is kidnapped out of a virtual reality performance. First in a new techno-thriller series.",
		URL:   "https://www.dungeonbooks.com/product/all-that-we-see-or-seem-a-julia-z-novel-by-ken-liu-hardcover-/TNF5WQKYFP5GXUFADKPFV463"},
	{ISBN: "9781534437685", BookTitle: "Black Sun", Author: "Rebecca Roanhorse", Collection: collBookClub, Month: "2025-11",
		Format: "paperback", Pages: 496, WeightGrams: 476,
		Blurb: "In a world drawn from the pre-Columbian Americas, a priest, a ship captain, and a young man bound for vengeance converge on the holy city of Tova for the winter solstice. First of Between Earth and Sky.",
		URL:   "https://www.dungeonbooks.com/product/black-sun-between-earth-and-sky-1-by-rebecca-roanhorse-paperback-/STUIP7CVTJL3STGO3DK5BPKE"},
	{ISBN: "9780441007318", BookTitle: "The Left Hand of Darkness", Author: "Ursula K. Le Guin", Collection: collBookClub, Month: "2025-10",
		Format: "paperback, 50th anniversary edition", Pages: 336, WeightGrams: 268,
		Blurb: "A lone envoy is sent to Winter, an icebound planet whose people can be any sex, to bring it into an interstellar union. The politics are hard; understanding the one person who trusts him is harder.",
		URL:   "https://www.dungeonbooks.com/product/the-left-hand-of-darkness-50th-anniversary-edition-ace-science-fiction-by-ursula-k-le-guin/FROYJ277UGCHCG7DN26RCGTZ"},
	{ISBN: "9781250880055", BookTitle: "The Devils", Author: "Joe Abercrombie", Collection: collBookClub, Month: "2025-09",
		Format: "hardcover", Pages: 560, WeightGrams: 776,
		Blurb: "A mild-mannered priest is handed a crew of monsters, a vampire and a werewolf among them, and a holy mission to put a princess on the throne. Bloody, funny, and not remotely holy.",
		URL:   "https://www.dungeonbooks.com/product/the-devils-by-joe-abercrombie-hardcover-/KW64Z54OHVNAK4AHXJRJNBVW"},
	{ISBN: "9781638933656", BookTitle: "Teo's Durumi", Author: "Elaine U. Cho", Collection: collBookClub, Month: "2025-08",
		Format: "paperback", Pages: 352, WeightGrams: 454,
		Blurb: "Teo Anand, former ne'er-do-well, follows Ocean and her crew into the cloisters of the Moon. Sequel to Ocean's Godori.",
		URL:   "https://www.dungeonbooks.com/product/teo-s-durumi-alliance-book-2-by-elaine-u-cho/3Y36PDXDMLNX3V44BNJQQ2UH"},
	{ISBN: "9781250267665", BookTitle: "A Master of Djinn", Author: "P. Djèlí Clark", Collection: collBookClub, Month: "2025-07",
		Format: "paperback", Pages: 448, WeightGrams: 386,
		Blurb: "Cairo, 1912. Fatma el-Sha'arawi, youngest woman at the Ministry of Alchemy, Enchantments and Supernatural Entities, investigates the murder of a secret brotherhood by someone claiming to be a legendary sorcerer returned.",
		URL:   "https://www.dungeonbooks.com/product/a-master-of-djinn-by-p-dj-l-clark-paperback-/UZQK6AIPRJSI7X2OMCQ6RUTP"},
	{ISBN: "9780765397539", BookTitle: "All Systems Red", Author: "Martha Wells", Collection: collBookClub, Month: "2025-06",
		Format: "paperback", Pages: 160, WeightGrams: 136,
		Blurb: "A security android hacks its own governor module and would rather watch serials than talk to anyone. Then the survey team it guards starts getting killed. First of the Murderbot Diaries.",
		URL:   "https://www.dungeonbooks.com/product/the-murderbot-diaries-all-systems-red-by-martha-wells/LVXTAQMUT57J2KNYCGG6AS3E"},
	{ISBN: "9781915998149", BookTitle: "The Last Phi Hunter", Author: "Salinee Goldenberg", Collection: collBookClub, Month: "2025-05",
		Format: "paperback", Pages: 384, WeightGrams: 408,
		Blurb: "Ex hunts phi, the ghosts of Thai folklore, and works alone. A pregnant runaway talks him into escorting her through a haunted forest, and the job gets much bigger than either of them.",
		URL:   "https://www.dungeonbooks.com/product/the-last-phi-hunter-by-salinee-goldenberg-paperback-/ZGEZQZ2RXVF35UDOYLDIFUFW"},
	{ISBN: "9780593820247", BookTitle: "Dungeon Crawler Carl", Author: "Matt Dinniman", Collection: collBookClub, Month: "2025-04",
		Format: "hardcover", Pages: 464, WeightGrams: 635,
		Blurb: "Earth's buildings vanish overnight and the survivors are dropped into an alien game show dungeon. Carl goes in wearing boxers, with his ex-girlfriend's prize-winning cat.",
		URL:   "https://www.dungeonbooks.com/product/dungeon-crawler-carl-dungeon-crawler-carl-book-1-by-matt-dinniman-hardcover-/RG6MLGIM2SOUUN3GBBDHSMOO"},
	{ISBN: "9780756404741", BookTitle: "The Name of the Wind", Author: "Patrick Rothfuss", Collection: collBookClub, Month: "2025-03",
		Format: "mass market paperback", Pages: 736, WeightGrams: 340,
		Blurb: "Kvothe, now an innkeeper in hiding, tells the true story behind his legend: a childhood with travelling players, years on the streets, and his way into the University.",
		URL:   "https://www.dungeonbooks.com/product/the-name-of-the-wind-by-patrick-rothfuss/YW35O2WXOTVSVIEJDZFOU3B6"},
	{ISBN: "9781250824042", BookTitle: "Annihilation", Author: "Jeff VanderMeer", Collection: collBookClub, Month: "2025-02",
		Format: "paperback", Pages: 224, WeightGrams: 177,
		Blurb: "Four women go into Area X, a coastline nature has retaken, as the twelfth expedition. The first eleven went badly. First of the Southern Reach.",
		URL:   "https://www.dungeonbooks.com/product/annihilation-southern-reach-book-1-by-jeff-vandermeer-paperback-/LPY6LHIMJMORO7BE7P5WVRQS"},
	{ISBN: "9780063021433", BookTitle: "Babel", Author: "R.F. Kuang", Collection: collBookClub, Month: "2025-01",
		Format: "paperback", Pages: 560, WeightGrams: 408,
		Blurb: "Oxford, 1830s. Robin Swift is trained at the Royal Institute of Translation, whose silver-working magic powers the British Empire, and has to decide what he owes it.",
		URL:   "https://www.dungeonbooks.com/product/babel-by-r-f-kuang-paperback-/EBZMWRE6SXLQBMPG5IKGFUA4"},
	// Square's item for it has no variations, so it only ever links out.
	{ISBN: "9780811237857", BookTitle: "It Lasts Forever and Then It's Over", Author: "Anne de Marcken", Collection: collBookClub, Month: "2024-11",
		Format: "paperback", Pages: 160, WeightGrams: 159,
		Blurb: "A woman wakes up undead and sets out across a ruined landscape in search of the man she loved. Winner of the Ursula K. Le Guin Prize."},
	// No Format: Square calls this the paperback, Ingram says the ISBN is the
	// hardcover deluxe, and it is too old a pick to be worth settling.
	{ISBN: "9781782275848", BookTitle: "Carmilla", Author: "Sheridan Le Fanu", Collection: collBookClub, Month: "2024-10",
		Pages: 160, WeightGrams: 272,
		Blurb: "Laura lives quietly in an Austrian castle until a carriage crashes nearby and leaves behind a beautiful guest. The vampire story that came before Dracula.",
		URL:   "https://www.dungeonbooks.com/product/carmilla-by-sheridan-le-fanu-paperback-/320"},

	{ISBN: "9781982136468", BookTitle: "The Only Good Indians", Author: "Stephen Graham Jones", Collection: collHorror, Month: "2026-10",
		Format: "paperback", Pages: 336, WeightGrams: 281,
		Blurb: "Ten years after an elk hunt on land they had no right to, four Blackfeet men find something is hunting them back.",
		URL:   "https://www.dungeonbooks.com/product/the-only-good-indians-by-stephen-graham-jones-paperback-/24KJLMMKKH2FIH4MD2JEHG2B"},
	{ISBN: "9780593723142", BookTitle: "Incidents Around the House", Author: "Josh Malerman", Collection: collHorror, Month: "2026-09",
		Format: "paperback", Pages: 400, WeightGrams: 318,
		Blurb: "Eight-year-old Bela has a friend her parents can't see, who she calls Other Mommy. Other Mommy wants to be let into Bela's heart. From the author of Bird Box.",
		URL:   "https://www.dungeonbooks.com/product/incidents-around-the-house-by-josh-malerman-paperback-/JKHDK6KY4IS2IBK4JW72RXRI"},
	{ISBN: "9781645661269", BookTitle: "Molka", Author: "Monika Kim", Collection: collHorror, Month: "2026-08",
		Format: "hardcover", Pages: 304, WeightGrams: 386,
		Blurb: "A woman discovers she has been secretly filmed and goes after the men responsible, with a very sharp kitchen knife. From the author of The Eyes Are the Best Part.",
		URL:   "https://www.dungeonbooks.com/product/molka-by-monika-kim-hardcover-/DYEVCM6CXE2VVXSRRGVWORGW"},
	{ISBN: "9780593156582", BookTitle: "The Staircase in the Woods", Author: "Chuck Wendig", Collection: collHorror, Month: "2026-07",
		Format: "paperback", Pages: 400, WeightGrams: 318,
		Blurb: "Five teenage friends found a staircase standing alone in the woods, and one of them climbed it and never came back. Twenty years later the survivors go back in.",
		URL:   "https://www.dungeonbooks.com/product/the-staircase-in-the-woods-by-chuck-wendig-paperback-/BEYUFXRLI6KAW5VKFPCFBPHC"},
	{ISBN: "9781982188351", BookTitle: "The Reformatory", Author: "Tananarive Due", Collection: collHorror, Month: "2026-06",
		Format: "paperback", Pages: 576, WeightGrams: 454,
		Blurb: "Jim Crow Florida, 1950. Twelve-year-old Robbie is sent to a segregated reform school where boys go missing, and where he starts to see their ghosts.",
		URL:   "https://www.dungeonbooks.com/product/the-reformatory-by-tananarive-due-paperback-/MWY4HE6YVKQ5JXF75O2ITUPN"},
	{ISBN: "9781335001559", BookTitle: "Japanese Gothic", Author: "Kylie Lee Baker", Collection: collHorror, Month: "2026-05",
		Format: "hardcover", Pages: 352, WeightGrams: 363,
		Blurb: "Two people with a claim on the same old house in Japan, and a history of the house that neither of them wants to hear, woven through with Japanese myth.",
		URL:   "https://www.dungeonbooks.com/product/japanese-gothic-by-kylie-lee-baker-hardcover-/5DLU7EKKKRYBMMJFTI4UA3QN"},
	{ISBN: "9781668068410", BookTitle: "Nothing Tastes as Good", Author: "Luke Dumas", Collection: collHorror, Month: "2026-04",
		Format: "hardcover", Pages: 352, WeightGrams: 499,
		Blurb: "Emmett signs up for a weight loss treatment and finally gets the body he wanted. The side effect is a hunger for something much worse.",
		URL:   "https://www.dungeonbooks.com/product/nothing-tastes-as-good-by-luke-dumas-hardcover-/ENBGRMFZBRANPUPQ5PODEBYU"},
	{ISBN: "9781250860057", BookTitle: "Nowhere Burning", Author: "Catriona Ward", Collection: collHorror, Month: "2026-03",
		Format: "hardcover", Pages: 304, WeightGrams: 481,
		Blurb: "Riley breaks her little brother out to a house in the Colorado mountains where runaway kids live wild. Something in the woods has been waiting for them.",
		URL:   "https://www.dungeonbooks.com/product/nowhere-burning-by-catriona-ward-hardcover-/RRAX62DUPUUI4AIEMG4EMPP7"},
	{ISBN: "9781250874696", BookTitle: "Red Rabbit", Author: "Alex Grecian", Collection: collHorror, Month: "2026-02",
		Format: "paperback", Pages: 480, WeightGrams: 431,
		Blurb: "A posse hunts a witch across a Kansas prairie full of demons and ghosts, where death is always just around the bend.",
		URL:   "https://www.dungeonbooks.com/product/red-rabbit-by-alex-grecian-paperback-/QFZ45JTZ6LFC2DCKHZKCNJMG"},
	{ISBN: "9781982198794", BookTitle: "We Used to Live Here", Author: "Marcus Kliewer", Collection: collHorror, Month: "2026-01",
		Format: "paperback", Pages: 320, WeightGrams: 181,
		Blurb: "A couple flipping an old house let the family that used to live there come in for a quick look. The visit doesn't end.",
		URL:   "https://www.dungeonbooks.com/product/we-used-to-live-here-by-marcus-kliewer-paperback-/D7TDCM7MRJAYNH25PBT5TLHX"},
	{ISBN: "9780593983751", BookTitle: "There Is No Antimemetics Division", Author: "qntm", Collection: collHorror, Month: "2025-12",
		Format: "hardcover", Pages: 288, WeightGrams: 386,
		Blurb: "The ideas that attack you here are antimemes: the moment you learn about one, you forget it. A small team fights them without remembering the war.",
		URL:   "https://www.dungeonbooks.com/product/there-is-no-antimemetics-division-by-qntm-hardcover-/J2OPD72QDOM4TF32MB7HNIJX"},
	{ISBN: "9780593548011", BookTitle: "The Hong Kong Widow", Author: "Kristen Loesch", Collection: collHorror, Month: "2025-11",
		Format: "hardcover", Pages: 368, WeightGrams: 522,
		Blurb: "Hong Kong, 1953. Witnesses swear a massacre happened in a remote mansion; the police find spotless rooms and call it a shared hallucination. Decades later one witness returns.",
		URL:   "https://www.dungeonbooks.com/product/the-hong-kong-widow-by-kristen-loesch-hardcover-/APX5ZYONKOQQM2UPQT3YR7SH"},
	{ISBN: "9780593874325", BookTitle: "The Bewitching", Author: "Silvia Moreno-Garcia", Collection: collHorror, Month: "2025-10",
		Format: "hardcover", Pages: 368, WeightGrams: 540,
		Blurb: "Three women in three eras, from 1900s Mexico to a 1990s New England college, and the witchcraft that links them. From the author of Mexican Gothic.",
		URL:   "https://www.dungeonbooks.com/product/the-bewitching-by-silvia-moreno-garcia-hardcover-/EMBUJZUVMBM2HONNBA6MAZEA"},
}

// featured is the index of the pick the shop opens on: the newest Sci-Fi &
// Fantasy pick, which is the top entry. Announcing a pick is what makes it
// current, and we do that a little before its month starts, so adding it
// here features it, rather than it waiting on the calendar to roll over.
//
// This deliberately does not consult the date. Matching Month against today
// left a pick announced early unfeatured until the 1st, which is the window we
// most want it visible in. The cost is that a shelf nobody has updated keeps
// featuring its last pick instead of falling back to nothing, which beats a
// shop that opens on no book at all.
//
// -1 only when there is no shelf.
func featured() int {
	return featuredIn(catalog)
}

// featuredIn is featured() over any shelf, so the API can answer it for the
// shelf it was given rather than the package one.
func featuredIn(books []Book) int {
	if len(books) == 0 {
		return -1
	}
	return 0
}

// section is the list heading a book sits under. The featured pick is lifted
// out into its own "featured" section, the way terminal.shop separates its
// featured product from the rest of the shelf.
func section(i int) string {
	if i == featured() {
		return collFeatured
	}
	return catalog[i].Collection
}
