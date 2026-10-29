using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
public class Odd
{
    [Key(0)]
    public string Name { get; set; }

    [Key(8)]
    public double OddsActual { get; set; }

    [Key(4)]
    public long Timestamp { get; set; }
}
