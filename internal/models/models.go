package models

type ObjectType int

const (
    ItemIsFile ObjectType = iota
    ItemIsDir
    ItemIsSymlink
)

type EventRecord struct {
    Filename   string
    Path       string
    ObjectType ObjectType
    EventID    uint64
}
