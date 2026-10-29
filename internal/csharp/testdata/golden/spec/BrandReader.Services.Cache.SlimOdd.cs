using MessagePack;

namespace BrandReader.Services.Cache;

[MessagePackObject]
public class SlimOdd
{
    [Key(0)]
    public string Name { get; set; }

    [Key(4)]
    public long Timestamp { get; set; }
}
