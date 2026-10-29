using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
public struct Venue
{
    [Key(1)]
    public long Id { get; set; }

    [Key(2)]
    public string Name { get; set; }
}
