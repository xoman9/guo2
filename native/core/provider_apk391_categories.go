package core

// Optional fallback categories for the selected sources; live navigation
// is always queried first and no remote catalogs are prefetched on startup.
var apk391StaticCategoryFallback = map[string][]nativeCategory{
    "chengguodj": {
        {ID:"yuanchuang",Name:"原創短劇"},
        {ID:"aiduanju",Name:"AI短劇"},
        {ID:"zhenren",Name:"真人短劇"},
        {ID:"mogai",Name:"AI魔改"},
        {ID:"manju",Name:"AI漫劇"},
    },
}
