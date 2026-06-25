package main

// AffiliateID is dungeonbooks' Bookshop.org affiliate identifier (matches
// marty's BOOKSHOP_AFFILIATE_ID). Affiliate links pay a commission; every ISBN
// below was validated against bookshop.org/book/{isbn} (308 = live).
const AffiliateID = "108216"

func affiliate(isbn string) string {
	return "https://bookshop.org/a/" + AffiliateID + "/" + isbn
}

// Book is one catalog entry. Affiliate model: no inventory, no payment — "buy"
// hands off to Bookshop.org. Free titles are openly licensed and read directly.
type Book struct {
	ISBN        string
	BookTitle   string
	Author      string
	Year        int
	Publisher   string
	Collection  string
	Blurb       string
	Free        bool
	DownloadURL string
}

func (b Book) BuyURL() string { return affiliate(b.ISBN) }

const (
	collCraft   = "the craft"
	collSystems = "systems & ai"
	collThink   = "how to think"
	collWorlds  = "other worlds"
)

// catalog is a deliberately small, curated shelf. Books people should actually
// read now — not a dump of the canon. Grouped by theme; fiction included
// because the best programmers read widely. Ordered by collection so the list
// groups cleanly.
var catalog = []Book{
	// --- the craft ---
	{ISBN: "9781098151249", BookTitle: "Tidy First?", Author: "Kent Beck", Year: 2023, Publisher: "O'Reilly", Collection: collCraft,
		Blurb: "On small structural changes to code (tidyings) and how to sequence them around behavior changes. Covers the individual tidyings, managing them inside larger changes, and the underlying theory."},
	{ISBN: "9780135957059", BookTitle: "The Pragmatic Programmer", Author: "Hunt & Thomas", Year: 2019, Publisher: "Addison-Wesley", Collection: collCraft,
		Blurb: "A general guide to software craft: coding, debugging, tooling, testing, and working on teams. The 20th anniversary edition, revised from the 1999 original."},
	{ISBN: "9780990582939", BookTitle: "Crafting Interpreters", Author: "Robert Nystrom", Year: 2021, Publisher: "Genever Benning", Collection: collCraft,
		Blurb: "Builds two interpreters for a scripting language called Lox: a tree-walk interpreter in Java and a bytecode virtual machine in C. Covers scanning, parsing, and garbage collection. Full text is free online.",
		Free:  true, DownloadURL: "https://craftinginterpreters.com"},

	// --- systems & ai ---
	{ISBN: "9781098119065", BookTitle: "Designing Data-Intensive Applications", Author: "Martin Kleppmann", Year: 2017, Publisher: "O'Reilly", Collection: collSystems,
		Blurb: "How data systems work: storage engines, data encoding, replication, partitioning, transactions, and consensus in distributed systems."},
	{ISBN: "9781098166304", BookTitle: "AI Engineering", Author: "Chip Huyen", Year: 2025, Publisher: "O'Reilly", Collection: collSystems,
		Blurb: "Building applications on top of foundation models: evaluation, prompt engineering, retrieval-augmented generation, finetuning, and inference optimization."},
	{ISBN: "9781633437166", BookTitle: "Build a Large Language Model (From Scratch)", Author: "Sebastian Raschka", Year: 2024, Publisher: "Manning", Collection: collSystems,
		Blurb: "Implements a GPT-style language model in PyTorch step by step, from tokenization and attention through pretraining and finetuning."},

	// --- how to think ---
	{ISBN: "9780465026562", BookTitle: "Godel, Escher, Bach", Author: "Douglas Hofstadter", Year: 1999, Publisher: "Basic Books", Collection: collThink,
		Blurb: "On how self-reference and formal systems relate to minds and meaning, built around Godel's incompleteness theorem, Escher's art, and Bach's music. Won the 1980 Pulitzer for general nonfiction."},
	{ISBN: "9781732265172", BookTitle: "The Art of Doing Science and Engineering", Author: "Richard Hamming", Year: 2020, Publisher: "Stripe Press", Collection: collThink,
		Blurb: "Based on Hamming's course at the Naval Postgraduate School on how to do significant research and engineering. Covers his career at Bell Labs and his approach to choosing problems."},
	{ISBN: "9781098118730", BookTitle: "The Staff Engineer's Path", Author: "Tanya Reilly", Year: 2022, Publisher: "O'Reilly", Collection: collThink,
		Blurb: "On engineering roles past the senior level: setting technical direction, leading projects, and operating as a technical leader without managing people."},

	// --- other worlds ---
	{ISBN: "9780553380958", BookTitle: "Snow Crash", Author: "Neal Stephenson", Year: 1992, Publisher: "Bantam Spectra", Collection: collWorlds,
		Blurb: "Cyberpunk novel. A hacker and pizza courier tracks a drug and computer virus called Snow Crash, moving between a privatized near-future US and a virtual world called the Metaverse."},
	{ISBN: "9780060512804", BookTitle: "Cryptonomicon", Author: "Neal Stephenson", Year: 1999, Publisher: "Avon", Collection: collWorlds,
		Blurb: "Novel with two timelines: codebreakers and cryptographers in World War II, and their descendants in the 1990s building a data haven. Includes real cryptography."},
	{ISBN: "9780765382030", BookTitle: "The Three-Body Problem", Author: "Cixin Liu", Year: 2014, Publisher: "Tor", Collection: collWorlds,
		Blurb: "Hard science fiction, translated by Ken Liu. Humanity makes first contact with a civilization from a chaotic three-star system. First book of the Remembrance of Earth's Past trilogy."},
	{ISBN: "9781597805391", BookTitle: "Permutation City", Author: "Greg Egan", Year: 1994, Publisher: "Night Shade", Collection: collWorlds,
		Blurb: "Science fiction about copies of human minds run as computer simulations, and a project to build a self-sustaining simulated universe."},
	{ISBN: "9780765311788", BookTitle: "Mistborn: The Final Empire", Author: "Brandon Sanderson", Year: 2006, Publisher: "Tor", Collection: collWorlds,
		Blurb: "Fantasy, first book of the Mistborn series. Set in a world ruled by an immortal emperor, where some people gain powers by ingesting and burning specific metals."},
}
