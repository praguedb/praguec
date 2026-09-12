using MessagePack;

namespace Common.IntegratedEventApi.Cache.Prague.Models;

[MessagePackObject]
public struct Venue
{
    [Key(1)]
    public required long Id { get; set; }
    [Key(2)]
    public required string Name { get; set; }
}